package notebookspawner

import (
	"bitbucket.org/linkernetworks/aurora/src/entity"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const NotebookContainerPort = 8888

type NotebookPodFactory struct {
	Notebook *entity.Notebook
}

type NotebookPodParameters struct {
	// Notebook parameters
	WorkingDir   string
	WorkspaceDir string
	Image        string
	BaseURL      string
	Bind         string
	Port         int32
	Labels       map[string]string
}

func (nb *NotebookPodFactory) DeploymentID() string {
	return nb.Notebook.ID.Hex()
}

func (nb *NotebookPodFactory) NewPod(podName string, params NotebookPodParameters) v1.Pod {
	return v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   podName,
			Labels: params.Labels,
		},
		Spec: v1.PodSpec{
			RestartPolicy: "Never",
			Containers: []v1.Container{
				{
					Image:           params.Image,
					Name:            podName,
					ImagePullPolicy: v1.PullPolicy("IfNotPresent"),
					Args: []string{
						"start-notebook.sh",
						"--notebook-dir=" + params.WorkingDir,
						"--ip=" + params.Bind,
						"--port=" + strconv.Itoa(int(params.Port)),
						"--NotebookApp.base_url=" + params.BaseURL,
						"--NotebookApp.token=''",
						"--NotebookApp.allow_origin='*'",
						"--NotebookApp.disable_check_xsrf=True",
						"--Session.debug=True",
					},
					VolumeMounts: []v1.VolumeMount{
						{Name: "data-volume", SubPath: params.WorkspaceDir, MountPath: params.WorkingDir},
					},
					Ports: []v1.ContainerPort{
						{
							Name:          "notebook-port",
							ContainerPort: params.Port,
							Protocol:      v1.ProtocolTCP,
						},
					},
					Env: []v1.EnvVar{
						{
							Name:  "CPU_GUARANTEE",
							Value: "200m",
						},
						{
							Name:  "MEM_GUARANTEE",
							Value: "256Mi",
						},
					},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							"cpu":    resource.MustParse("200m"),
							"memory": resource.MustParse("256Mi"),
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "data-volume",
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "data-storage",
						},
					},
				},
			},
		},
	}
}
