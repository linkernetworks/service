package notebookspawner

import (
	"bitbucket.org/linkernetworks/aurora/src/config"
	"bitbucket.org/linkernetworks/aurora/src/entity"
	"bitbucket.org/linkernetworks/aurora/src/kubemon"
	"bitbucket.org/linkernetworks/aurora/src/kubernetes/podtracker"
	"bitbucket.org/linkernetworks/aurora/src/service/kubernetes"
	"bitbucket.org/linkernetworks/aurora/src/service/mongo"
	"k8s.io/apimachinery/pkg/api/errors"

	"gopkg.in/mgo.v2/bson"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const PodNamePrefix = "notebook-"

// Object as Pod
type PodFactory interface {
	NewPod(podName string) v1.Pod
}

type PodLabelProvider interface {
	PodLabels() map[string]string
}

type ProxyInfoProvider interface {
	Host() string
	Port() string
	BaseURL() string
}

type DeploymentIDProvider interface {
	DeploymentID() string
}

type PodDeployment interface {
	DeploymentIDProvider
	PodFactory
}

type NotebookPodDeployment interface {
	PodDeployment
	ProxyInfoProvider
}

type NotebookSpawnerService struct {
	Config     config.Config
	Mongo      *mongo.MongoService
	Context    *mongo.Context
	Kubernetes *kubernetes.Service
	namespace  string
}

func New(c config.Config, m *mongo.MongoService, k *kubernetes.Service) *NotebookSpawnerService {
	// FIXME: provide method to free context
	return &NotebookSpawnerService{c, m, m.NewContext(), k, "default"}
}

func (s *NotebookSpawnerService) Sync(notebookID bson.ObjectId, pod *v1.Pod) error {
	backend := entity.ProxyBackend{
		// TODO: extract this as the service configuration
		IP:        pod.Status.PodIP,
		Port:      NotebookContainerPort,
		Connected: pod.Status.PodIP != "",
	}

	podInfo := entity.PodInfo{
		Phase:     pod.Status.Phase,
		Message:   pod.Status.Message,
		Reason:    pod.Status.Reason,
		StartTime: pod.Status.StartTime,
	}

	q := bson.M{"_id": notebookID}
	m := bson.M{
		"$set": bson.M{
			"backend": backend,
			"pod":     podInfo,
		},
	}
	return s.Context.C(entity.NotebookCollectionName).Update(q, m)
}

func (s *NotebookSpawnerService) DeployPod(notebook PodDeployment) error {
	return nil
}

func (s *NotebookSpawnerService) Start(nb *entity.Notebook) (*podtracker.PodTracker, error) {
	clientset, err := s.Kubernetes.CreateClientset()
	if err != nil {
		return nil, err
	}

	workspace := entity.Workspace{}
	err = s.Context.FindOne(entity.WorkspaceCollectionName, bson.M{"_id": nb.WorkspaceID}, &workspace)
	if err != nil {
		return nil, err
	}

	// TODO: load workspace to ensure the workspace exists
	// workspace := filepath.Join(s.Config.Data.BatchDir, "batch-"+nb.WorkspaceID.Hex())
	podName := PodNamePrefix + nb.DeploymentID()

	podFactory := NotebookPodFactory{nb}
	pod := podFactory.NewPod(podName, NotebookPodParameters{
		Image:        nb.Image,
		WorkspaceDir: workspace.Directory,
		WorkingDir:   "/batch",
		Bind:         "0.0.0.0",
		Port:         NotebookContainerPort,
		BaseURL:      nb.Url,
	})

	_, err = kubemon.FindPod(clientset, s.namespace, podName)
	if errors.IsNotFound(err) {
		// Pod not found. Start pod for notebook in workspace(batch)
		_, err = clientset.Core().Pods(s.namespace).Create(&pod)
		if err != nil {
			return nil, err
		}
		podTracker := s.startTracking(clientset, podName, nb)
		return podTracker, nil
	} else if err != nil {
		return nil, err
	}

	podTracker := s.startTracking(clientset, podName, nb)
	return podTracker, nil
}

func (s *NotebookSpawnerService) Stop(nb *entity.Notebook) (*podtracker.PodTracker, error) {
	clientset, err := s.Kubernetes.CreateClientset()
	if err != nil {
		return nil, err
	}

	podName := PodNamePrefix + nb.DeploymentID()
	err = clientset.Core().Pods(s.namespace).Delete(podName, metav1.NewDeleteOptions(0))
	if err != nil {
		return nil, err
	}

	podTracker := s.startTracking(clientset, podName, nb)
	return podTracker, nil
}
