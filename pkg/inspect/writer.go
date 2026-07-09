package inspect

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/client-go/rest"
)

const (
	filePermission   = 0644
	folderPermission = os.ModePerm
)

func writeYAML(filepath string, obj runtime.Object) error {
	dir := path.Dir(filepath)
	if err := os.MkdirAll(dir, folderPermission); err != nil {
		return fmt.Errorf("unable to create dir %s: %w", dir, err)
	}

	f, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("unable to create file %s: %w", filepath, err)
	}
	defer f.Close()

	printer := printers.YAMLPrinter{}
	return printer.PrintObj(obj, f)
}

func writeLogFromRequest(filepath string, req *rest.Request) error {
	dir := path.Dir(filepath)
	if err := os.MkdirAll(dir, folderPermission); err != nil {
		return fmt.Errorf("unable to create dir %s: %w", dir, err)
	}

	stream, err := req.Stream(context.TODO())
	if err != nil {
		return fmt.Errorf("unable to stream logs to %s: %w", filepath, err)
	}
	defer stream.Close()

	f, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("unable to create file %s: %w", filepath, err)
	}
	defer f.Close()

	_, err = io.Copy(f, stream)
	return err
}
