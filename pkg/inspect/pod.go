package inspect

import (
	"path"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
)

func gatherPodData(kubeClient kubernetes.Interface, destDir string, pod *corev1.Pod) error {
	podDir := path.Join(destDir, pod.Name)

	if err := writeYAML(path.Join(podDir, pod.Name+".yaml"), pod); err != nil {
		return err
	}

	var errs []error
	for _, container := range pod.Spec.Containers {
		if err := gatherContainerLogs(kubeClient, podDir, pod, container); err != nil {
			errs = append(errs, err)
		}
	}
	for _, container := range pod.Spec.InitContainers {
		if err := gatherContainerLogs(kubeClient, podDir, pod, container); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.NewAggregate(errs)
}

func gatherContainerLogs(kubeClient kubernetes.Interface, podDir string, pod *corev1.Pod, container corev1.Container) error {
	logsDir := path.Join(podDir, container.Name, container.Name, "logs")

	var errs []error
	wg := sync.WaitGroup{}
	errLock := sync.Mutex{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		var innerErrs []error

		logOpts := &corev1.PodLogOptions{
			Container:  container.Name,
			Follow:     false,
			Previous:   false,
			Timestamps: true,
		}
		req := kubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOpts)
		if err := writeLogFromRequest(path.Join(logsDir, "current.log"), req); err != nil {
			innerErrs = append(innerErrs, err)
			logOpts.InsecureSkipTLSVerifyBackend = true
			req = kubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOpts)
			if err := writeLogFromRequest(path.Join(logsDir, "current.insecure.log"), req); err != nil {
				innerErrs = append(innerErrs, err)
			}
		}

		errLock.Lock()
		defer errLock.Unlock()
		errs = append(errs, innerErrs...)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		var innerErrs []error

		logOpts := &corev1.PodLogOptions{
			Container:  container.Name,
			Follow:     false,
			Previous:   true,
			Timestamps: true,
		}
		req := kubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOpts)
		if err := writeLogFromRequest(path.Join(logsDir, "previous.log"), req); err != nil {
			if !isPreviousContainerNotFound(err) {
				innerErrs = append(innerErrs, err)
				logOpts.InsecureSkipTLSVerifyBackend = true
				req = kubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOpts)
				if err := writeLogFromRequest(path.Join(logsDir, "previous.insecure.log"), req); err != nil {
					innerErrs = append(innerErrs, err)
				}
			}
		}

		errLock.Lock()
		defer errLock.Unlock()
		errs = append(errs, innerErrs...)
	}()

	wg.Wait()
	return errors.NewAggregate(errs)
}

func isPreviousContainerNotFound(err error) bool {
	return strings.Contains(err.Error(), "previous terminated container") &&
		strings.HasSuffix(err.Error(), "not found")
}
