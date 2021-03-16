package klient

import (
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/kubectl/pkg/util"
)

// Apply creates a resource with the given content
func (c *Client) Apply(content []byte) error {
	opt := BuilderOptions{Validate: false, Unstructured: true}
	r := c.ResultForContent(content, &opt)
	return c.ApplyResource(r)
}

// ApplyFiles create the resource(s) from the given filenames (file, directory or STDIN) or HTTP URLs
func (c *Client) ApplyFiles(filenames ...string) error {
	r := c.ResultForFilenameParam(filenames, nil)
	return c.ApplyResource(r)
}

// ApplyResource applies the given resource. Create the resources with `ResultForFilenameParam` or `ResultForContent`
func (c *Client) ApplyResource(r *resource.Result) error {
	if err := r.Err(); err != nil {
		return err
	}

	// Is ServerSideApply requested
	if c.ServerSideApply {
		return r.Visit(serverSideApply)
	}

	return r.Visit(apply)
}

var ChartReleaseRevisionKey = "asm.revision"

func apply(info *resource.Info, err error) error {
	if err != nil {
		return failedTo("apply", info, err)
	}

	// If it does not exists, just create it
	current, err := resource.NewHelper(info.Client, info.Mapping).Get(info.Namespace, info.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return failedTo("retrieve current configuration", info, err)
		}
		if err := util.CreateApplyAnnotation(info.Object, unstructured.UnstructuredJSONScheme); err != nil {
			return failedTo("set annotation", info, err)
		}
		return create(info, nil)
	}
	needPatch := true
	if obj, ok := current.(metav1.Object); ok {
		labels := obj.GetLabels()
		if cV, cExists := labels[ChartReleaseRevisionKey]; cExists {
			if newObj, ok := info.Object.(metav1.Object); ok {
				newLabels := newObj.GetLabels()
				if nV, nExists := newLabels[ChartReleaseRevisionKey]; nExists {
					if cV == nV {
						needPatch = false
					}
				}
			}
		}
	}
	// If exists, patch it
	if needPatch {
		return patch(info, current)
	}
	return nil
}

func serverSideApply(info *resource.Info, err error) error {
	if err != nil {
		return failedTo("serverside apply", info, err)
	}

	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, info.Object)
	if err != nil {
		return failedTo("encode for the serverside apply", info, err)
	}

	options := metav1.PatchOptions{
		// TODO: Find out how to get the force conflict flag
		// Force:        &forceConflicts,
		// FieldManager: FieldManager,
	}
	obj, err := resource.NewHelper(info.Client, info.Mapping).Patch(info.Namespace, info.Name, types.ApplyPatchType, data, &options)
	if err != nil {
		return failedTo("serverside patch", info, err)
	}
	info.Refresh(obj, true)
	return nil
}
