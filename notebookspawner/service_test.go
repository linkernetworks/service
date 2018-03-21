package notebookspawner

import (
	"os"
	"testing"

	"bitbucket.org/linkernetworks/aurora/src/config"
	"bitbucket.org/linkernetworks/aurora/src/entity"
	"bitbucket.org/linkernetworks/aurora/src/service/kubernetes"
	"bitbucket.org/linkernetworks/aurora/src/service/mongo"
	"bitbucket.org/linkernetworks/aurora/src/service/redis"

	"bitbucket.org/linkernetworks/aurora/src/workspace"

	v1 "k8s.io/api/core/v1"

	// "bitbucket.org/linkernetworks/aurora/src/service/notebookspawner/notebook"
	"github.com/stretchr/testify/assert"
	"gopkg.in/mgo.v2/bson"
)

const (
	testingConfigPath = "../../../config/testing.json"
)

func TestNotebookSpawnerService(t *testing.T) {
	if _, defined := os.LookupEnv("TEST_K8S"); !defined {
		t.SkipNow()
		return
	}

	var notebookImage = "jupyter/minimal-notebook"
	var err error

	//Get mongo service
	cf := config.MustRead(testingConfigPath)

	kubernetesService := kubernetes.NewFromConfig(cf.Kubernetes)
	mongoService := mongo.New(cf.Mongo.Url)
	redisService := redis.New(cf.Redis)

	clientset, err := kubernetesService.NewClientset()
	assert.NoError(t, err)

	spawner := New(cf, mongoService, clientset, redisService)

	// proxyURL := "/v1/notebooks/proxy/"
	session := mongoService.NewSession()
	defer session.Close()

	userId := bson.NewObjectId()

	ws := entity.Workspace{
		ID:    bson.NewObjectId(),
		Name:  "testing workspace",
		Type:  "general",
		Owner: userId,
	}

	err = session.C(entity.WorkspaceCollectionName).Insert(ws)
	assert.NoError(t, err)
	defer session.C(entity.WorkspaceCollectionName).Remove(bson.M{"_id": ws.ID})

	// ensure that the primary volume is created
	err = workspace.PrepareVolume(session, &ws, kubernetesService)
	assert.NoError(t, err)
	assert.NotNil(t, ws.PrimaryVolume)

	notebookID := bson.NewObjectId()
	notebook := entity.Notebook{
		ID:          notebookID,
		Image:       notebookImage,
		WorkspaceID: ws.ID,
		Url:         cf.Jupyter.BaseURL + "/" + notebookID.Hex(),
		CreatedBy:   userId,
	}
	err = session.C(entity.NotebookCollectionName).Insert(notebook)
	assert.NoError(t, err)
	defer session.C(entity.NotebookCollectionName).Remove(bson.M{"_id": notebook.ID})

	pod, err := spawner.NewPod(&notebook)
	assert.NoError(t, err)

	for _, v := range pod.Spec.Volumes {
		t.Logf("Added Volume: %s", v.Name)
	}
	for _, m := range pod.Spec.Containers[0].VolumeMounts {
		if len(m.SubPath) == 0 {
			t.Logf("Added Mount: mount %s at %s", m.Name, m.MountPath)
		} else {
			t.Logf("Added Mount: mount %s from %s at %s", m.Name, m.SubPath, m.MountPath)
		}
	}

	assert.Equal(t, 2, len(pod.Spec.Volumes))
	assert.Equal(t, 2, len(pod.Spec.Containers[0].VolumeMounts))

	t.Logf("starting notebook: %v", notebook.ID)
	tracker, err := spawner.Start(&ws, &notebook)
	assert.NoError(t, err)

	t.Logf("waiting for pod phase")
	tracker.WaitForPhase(v1.PodPhase("Running"))

	t.Logf("syncing notebook document")
	err = spawner.Updater.Sync(&notebook)
	assert.NoError(t, err)

	_, err = spawner.Stop(&ws, &notebook)
	assert.NoError(t, err)

	err = spawner.Updater.Sync(&notebook)
	assert.NoError(t, err)

}
