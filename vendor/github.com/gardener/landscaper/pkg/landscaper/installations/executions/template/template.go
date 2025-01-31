// SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors.
//
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"encoding/json"
	"fmt"

	cdv2 "github.com/gardener/component-spec/bindings-go/apis/v2"
	"github.com/gardener/component-spec/bindings-go/codec"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/gardener/landscaper/pkg/api"
	"github.com/gardener/landscaper/pkg/utils"

	"github.com/gardener/landscaper/apis/core"
	"github.com/gardener/landscaper/apis/core/validation"

	lsv1alpha1 "github.com/gardener/landscaper/apis/core/v1alpha1"
	"github.com/gardener/landscaper/pkg/landscaper/blueprints"
)

// Templater implements all available template executors.
type Templater struct {
	impl map[lsv1alpha1.TemplateType]ExecutionTemplater
}

// New creates a new instance of a templater.
func New(templaters ...ExecutionTemplater) *Templater {
	t := &Templater{
		impl: make(map[lsv1alpha1.TemplateType]ExecutionTemplater),
	}
	for _, templater := range templaters {
		t.impl[templater.Type()] = templater
	}
	return t
}

// ExecutionTemplater describes a implementation for a template execution
type ExecutionTemplater interface {
	// Type returns the type of the templater.
	Type() lsv1alpha1.TemplateType
	// TemplateSubinstallationExecutions templates a subinstallation executor and return a list of installations templates.
	TemplateSubinstallationExecutions(tmplExec lsv1alpha1.TemplateExecutor,
		blueprint *blueprints.Blueprint,
		cd *cdv2.ComponentDescriptor,
		cdList *cdv2.ComponentDescriptorList,
		values map[string]interface{}) (*SubinstallationExecutorOutput, error)
	// TemplateDeployExecutions templates a deploy executor and return a list of deployitem templates.
	TemplateDeployExecutions(tmplExec lsv1alpha1.TemplateExecutor,
		blueprint *blueprints.Blueprint,
		cd *cdv2.ComponentDescriptor,
		cdList *cdv2.ComponentDescriptorList,
		values map[string]interface{}) (*DeployExecutorOutput, error)
	// TemplateExportExecutions templates a export executor.
	// It return the exported data as key value map where the key is the name of the export.
	TemplateExportExecutions(tmplExec lsv1alpha1.TemplateExecutor,
		blueprint *blueprints.Blueprint,
		exports interface{}) (*ExportExecutorOutput, error)
}

// SubinstallationExecutorOutput describes the output of deploy executor.
type SubinstallationExecutorOutput struct {
	Subinstallations []*lsv1alpha1.InstallationTemplate `json:"subinstallations"`
}

func (o SubinstallationExecutorOutput) MarshalJSON() ([]byte, error) {
	return json.Marshal(o)
}

func (o *SubinstallationExecutorOutput) UnmarshalJSON(data []byte) error {
	type helperStruct struct {
		Subinstallations []json.RawMessage `json:"subinstallations"`
	}
	rawList := &helperStruct{}
	if err := json.Unmarshal(data, rawList); err != nil {
		return err
	}

	out := SubinstallationExecutorOutput{
		Subinstallations: make([]*lsv1alpha1.InstallationTemplate, len(rawList.Subinstallations)),
	}
	for i, raw := range rawList.Subinstallations {
		instTmpl := lsv1alpha1.InstallationTemplate{}
		if _, _, err := api.Decoder.Decode(raw, nil, &instTmpl); err != nil {
			return fmt.Errorf("unable to decode installation template %d: %w", i, err)
		}
		out.Subinstallations[i] = &instTmpl
	}

	*o = out
	return nil
}

// DeployExecutorOutput describes the output of deploy executor.
type DeployExecutorOutput struct {
	DeployItems []lsv1alpha1.DeployItemTemplate `json:"deployItems"`
}

// ExportExecutorOutput describes the output of export executor.
type ExportExecutorOutput struct {
	Exports map[string]interface{} `json:"exports"`
}

// DeployExecutionOptions describes the options for templating the deploy executions.
type DeployExecutionOptions struct {
	Imports map[string]interface{}
	// +optional
	Installation         *lsv1alpha1.Installation
	Blueprint            *blueprints.Blueprint
	ComponentDescriptor  *cdv2.ComponentDescriptor
	ComponentDescriptors *cdv2.ComponentDescriptorList
}

// TemplateSubinstallationExecutions templates all subinstallation executions and
// returns a aggregated list of all templated installation templates.
func (o *Templater) TemplateSubinstallationExecutions(opts DeployExecutionOptions) ([]*lsv1alpha1.InstallationTemplate, error) {
	// marshal and unmarshal resolved component descriptor
	component, err := serializeComponentDescriptor(opts.ComponentDescriptor)
	if err != nil {
		return nil, fmt.Errorf("error during serializing of the resolved components: %w", err)
	}
	components, err := serializeComponentDescriptorList(opts.ComponentDescriptors)
	if err != nil {
		return nil, fmt.Errorf("error during serializing of the component descriptor: %w", err)
	}

	values := map[string]interface{}{
		"imports":    opts.Imports,
		"cd":         component,
		"components": components,
	}

	// add blueprint and component descriptor ref information to the input values
	if opts.Installation != nil {
		blueprintDef, err := utils.JSONSerializeToGenericObject(opts.Installation.Spec.Blueprint)
		if err != nil {
			return nil, fmt.Errorf("unable to serialize the blueprint definition")
		}
		values["blueprint"] = blueprintDef

		if opts.Installation.Spec.ComponentDescriptor != nil {
			cdDef, err := utils.JSONSerializeToGenericObject(opts.Installation.Spec.ComponentDescriptor)
			if err != nil {
				return nil, fmt.Errorf("unable to serialize the component descriptor definition")
			}
			values["componentDescriptorDef"] = cdDef
		}
	}

	installationTemplates := make([]*lsv1alpha1.InstallationTemplate, 0)
	for _, tmplExec := range opts.Blueprint.Info.SubinstallationExecutions {
		impl, ok := o.impl[tmplExec.Type]
		if !ok {
			return nil, fmt.Errorf("unknown template type %s", tmplExec.Type)
		}

		output, err := impl.TemplateSubinstallationExecutions(tmplExec, opts.Blueprint, opts.ComponentDescriptor, opts.ComponentDescriptors, values)
		if err != nil {
			return nil, err
		}
		if output.Subinstallations == nil {
			continue
		}
		installationTemplates = append(installationTemplates, output.Subinstallations...)
	}

	return installationTemplates, nil
}

// TemplateDeployExecutions templates all deploy executions and returns a aggregated list of all templated deploy item templates.
func (o *Templater) TemplateDeployExecutions(opts DeployExecutionOptions) ([]lsv1alpha1.DeployItemTemplate, error) {

	// marshal and unmarshal resolved component descriptor
	component, err := serializeComponentDescriptor(opts.ComponentDescriptor)
	if err != nil {
		return nil, fmt.Errorf("error during serializing of the resolved components: %w", err)
	}
	components, err := serializeComponentDescriptorList(opts.ComponentDescriptors)
	if err != nil {
		return nil, fmt.Errorf("error during serializing of the component descriptor: %w", err)
	}

	values := map[string]interface{}{
		"imports":    opts.Imports,
		"cd":         component,
		"components": components,
	}

	// add blueprint and component descriptor ref information to the input values
	if opts.Installation != nil {
		blueprintDef, err := utils.JSONSerializeToGenericObject(opts.Installation.Spec.Blueprint)
		if err != nil {
			return nil, fmt.Errorf("unable to serialize the blueprint definition")
		}
		values["blueprint"] = blueprintDef

		if opts.Installation.Spec.ComponentDescriptor != nil {
			cdDef, err := utils.JSONSerializeToGenericObject(opts.Installation.Spec.ComponentDescriptor)
			if err != nil {
				return nil, fmt.Errorf("unable to serialize the component descriptor definition")
			}
			values["componentDescriptorDef"] = cdDef
		}
	}

	deployItemTemplateList := lsv1alpha1.DeployItemTemplateList{}
	for _, tmplExec := range opts.Blueprint.Info.DeployExecutions {
		impl, ok := o.impl[tmplExec.Type]
		if !ok {
			return nil, fmt.Errorf("unknown template type %s", tmplExec.Type)
		}

		output, err := impl.TemplateDeployExecutions(tmplExec, opts.Blueprint, opts.ComponentDescriptor, opts.ComponentDescriptors, values)
		if err != nil {
			return nil, err
		}
		if output.DeployItems == nil {
			continue
		}
		deployItemTemplateList = append(deployItemTemplateList, output.DeployItems...)
	}

	if err := validateDeployItemList(field.NewPath("deployExecutions"), deployItemTemplateList); err != nil {
		return nil, err
	}

	return deployItemTemplateList, nil
}

func validateDeployItemList(fldPath *field.Path, list lsv1alpha1.DeployItemTemplateList) error {
	coreList := core.DeployItemTemplateList{}
	if err := lsv1alpha1.Convert_v1alpha1_DeployItemTemplateList_To_core_DeployItemTemplateList(&list, &coreList, nil); err != nil {
		return err
	}
	return validation.ValidateDeployItemTemplateList(fldPath, coreList).ToAggregate()
}

// TemplateExportExecutions templates all deploy executions and returns a aggregated list of all templated deploy item templates.
func (o *Templater) TemplateExportExecutions(blueprint *blueprints.Blueprint, exports interface{}) (map[string]interface{}, error) {
	exportData := make(map[string]interface{})
	for _, tmplExec := range blueprint.Info.ExportExecutions {

		impl, ok := o.impl[tmplExec.Type]
		if !ok {
			return nil, fmt.Errorf("unknown template type %s", tmplExec.Type)
		}

		output, err := impl.TemplateExportExecutions(tmplExec, blueprint, exports)
		if err != nil {
			return nil, err
		}
		exportData = utils.MergeMaps(exportData, output.Exports)
	}

	return exportData, nil
}

func serializeComponentDescriptor(cd *cdv2.ComponentDescriptor) (interface{}, error) {
	if cd == nil {
		return nil, nil
	}
	data, err := codec.Encode(cd)
	if err != nil {
		return nil, err
	}
	var val interface{}
	if err := json.Unmarshal(data, &val); err != nil {
		return nil, err
	}
	return val, nil
}

func serializeComponentDescriptorList(cd *cdv2.ComponentDescriptorList) (interface{}, error) {
	if cd == nil {
		return nil, nil
	}
	data, err := codec.Encode(cd)
	if err != nil {
		return nil, err
	}
	var val interface{}
	if err := json.Unmarshal(data, &val); err != nil {
		return nil, err
	}
	return val, nil
}
