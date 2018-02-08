package fileserver

import (
	"bitbucket.org/linkernetworks/aurora/src/types/container"
	"strconv"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const WorkspacePodNamePrefix = "workspace-fs-"
const FileServerContainerPort = 33333
const FileServerImage = "gcr.io/linker-aurora/fileserver"

type FileServerPodFactory struct {
}

type FileServerPodParameters struct {
	// FileServer parameters
	Port    int32
	Image   string
	Labels  map[string]string
	Volumes []container.Volume
}

func getKubeVolume(params FileServerPodParameters) []v1.Volume {
	kubeVolume := []v1.Volume{}
	for _, v := range params.Volumes {
		kubeVolume = append(kubeVolume, v1.Volume{
			Name: v.Volume.Name,
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: v.ClaimName,
					},
				},
			},
		})
	}
	return kubeVolume
}

func getKubeVolumeMount(params FileServerPodParameters) []v1.VolumeMount {
	kubeVolumeMount := []v1.VolumeMount{}
	for _, v := range params.Volumes {
		kubeVolumeMount = append(kubeVolumeMount, v1.VolumeMount{
			Name:      v.Volume.Name,
			SubPath:   v.Volume.SubPath,
			MountPath: v.Volume.MountPath,
		})
	}
	return kubeVolumeMount
}

func (fs *FileServerPodFactory) NewPod(podName string, params FileServerPodParameters) v1.Pod {
	kubeVolume := getKubeVolume(params)
	kubeVolumeMount := getKubeVolumeMount(params)

	return v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   podName,
			Labels: params.Labels,
		},
		Spec: v1.PodSpec{
			RestartPolicy: "Always",
			Containers: []v1.Container{
				{
					Image:           params.Image,
					Name:            podName,
					ImagePullPolicy: v1.PullPolicy("IfNotPresent"),
					Args: []string{
						"-p " + strconv.Itoa(int(params.Port)),
					},
					VolumeMounts: kubeVolumeMount,
					Ports: []v1.ContainerPort{
						{
							Name:          "fileserver",
							ContainerPort: params.Port,
							Protocol:      v1.ProtocolTCP,
						},
					},
				},
			},
			Volumes: kubeVolume,
		},
	}
}
