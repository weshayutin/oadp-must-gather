package inspect

import (
	"context"
	"errors"
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

func writeYAML(filepath string, obj runtime.Object) (retErr error) {
	dir := path.Dir(filepath)
	if err := os.MkdirAll(dir, folderPermission); err != nil {
		return fmt.Errorf("unable to create dir %s: %w", dir, err)
	}

	f, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filePermission)
	if err != nil {
		return fmt.Errorf("unable to create file %s: %w", filepath, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && retErr == nil {
			retErr = closeErr
		}
	}()

	printer := printers.YAMLPrinter{}
	return printer.PrintObj(obj, f)
}

func writeLogFromRequest(ctx context.Context, filepath string, req *rest.Request) (retErr error) {
	dir := path.Dir(filepath)
	if err := os.MkdirAll(dir, folderPermission); err != nil {
		return fmt.Errorf("unable to create dir %s: %w", dir, err)
	}

	stream, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf("unable to stream logs to %s: %w", filepath, err)
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil && retErr == nil {
			retErr = closeErr
		}
	}()

	f, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filePermission)
	if err != nil {
		return fmt.Errorf("unable to create file %s: %w", filepath, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && retErr == nil {
			retErr = closeErr
		}
	}()

	_, err = io.Copy(f, stream)
	if errors.Is(err, io.ErrUnexpectedEOF) {
		fmt.Printf("  warning: partial log written to %s (unexpected EOF)\n", filepath)
		return nil
	}
	return err
}
