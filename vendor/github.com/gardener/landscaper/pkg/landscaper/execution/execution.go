// SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors.
//
// SPDX-License-Identifier: Apache-2.0

package execution

import (
	"context"

	"github.com/gardener/landscaper/pkg/utils/read_write_layer"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	lsv1alpha1 "github.com/gardener/landscaper/apis/core/v1alpha1"
	lsv1alpha1helper "github.com/gardener/landscaper/apis/core/v1alpha1/helper"
	"github.com/gardener/landscaper/pkg/api"
	"github.com/gardener/landscaper/pkg/landscaper/dataobjects"
	"github.com/gardener/landscaper/pkg/landscaper/operation"
)

// Operation contains all execution operations
type Operation struct {
	*operation.Operation
	exec           *lsv1alpha1.Execution
	forceReconcile bool
}

// NewOperation creates a new execution operations
func NewOperation(op *operation.Operation, exec *lsv1alpha1.Execution, forceReconcile bool) *Operation {
	return &Operation{
		Operation:      op,
		exec:           exec,
		forceReconcile: forceReconcile,
	}
}

// UpdateStatus updates the status of a execution
func (o *Operation) UpdateStatus(ctx context.Context, phase lsv1alpha1.ExecutionPhase, updatedConditions ...lsv1alpha1.Condition) error {
	o.exec.Status.Phase = phase
	o.exec.Status.Conditions = lsv1alpha1helper.MergeConditions(o.exec.Status.Conditions, updatedConditions...)
	if err := o.Writer().UpdateExecutionStatus(ctx, read_write_layer.W000032, o.exec); err != nil {
		o.Log().Error(err, "unable to set installation status")
		return err
	}
	return nil
}

// CreateOrUpdateExportReference creates or updates a dataobject from a object reference
func (o *Operation) CreateOrUpdateExportReference(ctx context.Context, values interface{}) error {
	do := dataobjects.New().
		SetNamespace(o.exec.Namespace).
		SetSource(lsv1alpha1helper.DataObjectSourceFromExecution(o.exec)).
		SetContext(lsv1alpha1helper.DataObjectSourceFromExecution(o.exec)).
		SetData(values)

	raw, err := do.Build()
	if err != nil {
		return err
	}

	if _, err := o.Writer().CreateOrUpdateDataObject(ctx, read_write_layer.W000075, raw, func() error {
		if err := controllerutil.SetOwnerReference(o.exec, raw, api.LandscaperScheme); err != nil {
			return err
		}
		return do.Apply(raw)
	}); err != nil {
		return err
	}

	o.exec.Status.ExportReference = &lsv1alpha1.ObjectReference{
		Name:      raw.Name,
		Namespace: raw.Namespace,
	}
	return o.UpdateStatus(ctx, o.exec.Status.Phase)
}
