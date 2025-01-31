// SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors.
//
// SPDX-License-Identifier: Apache-2.0

package components

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"text/template"

	"github.com/gardener/landscapercli/pkg/components"

	"github.com/gardener/landscaper/apis/core/v1alpha1"
	"github.com/gardener/landscaper/apis/deployer/utils/managedresource"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/yaml"

	"github.com/gardener/landscapercli/pkg/blueprints"
	"github.com/gardener/landscapercli/pkg/logger"
	"github.com/gardener/landscapercli/pkg/util"
)

const addManifestDeployItemUse = `deployitem \
    [deployitem name] \
   `

const addManifestDeployItemExample = `
landscaper-cli component add manifest deployitem \
  nginx \
  --component-directory ~/myComponent \
  --manifest-file ./deployment.yaml \
  --manifest-file ./service.yaml \
  --import-param replicas:integer
  --cluster-param target-cluster
`

const addManifestDeployItemShort = `
Command to add a deploy item skeleton to the blueprint of a component`

//var identityKeyValidationRegexp = regexp.MustCompile("^[a-z0-9]([-_+a-z0-9]*[a-z0-9])?$")

type addManifestDeployItemOptions struct {
	componentPath string

	deployItemName string

	// names of manifest files
	files *[]string

	// import parameter definitions in the format "name:type"
	importParams *[]string

	// parsed import parameter definitions
	importDefinitions map[string]*v1alpha1.ImportDefinition

	// a map that assigns with each import parameter name a uuid
	replacement map[string]string

	updateStrategy string

	policy string

	clusterParam string
}

// NewCreateCommand creates a new blueprint command to create a blueprint
func NewAddManifestDeployItemCommand(ctx context.Context) *cobra.Command {
	opts := &addManifestDeployItemOptions{}
	cmd := &cobra.Command{
		Use:     addManifestDeployItemUse,
		Example: addManifestDeployItemExample,
		Short:   addManifestDeployItemShort,
		Args:    cobra.ExactArgs(1),

		Run: func(cmd *cobra.Command, args []string) {
			if err := opts.Complete(args); err != nil {
				fmt.Println(err.Error())
				os.Exit(1)
			}

			if err := opts.run(ctx, logger.Log); err != nil {
				fmt.Println(err.Error())
				os.Exit(1)
			}

			fmt.Printf("Deploy item added")
			fmt.Printf("  \n- deploy item definition in blueprint folder in file %s created", util.ExecutionFileName(opts.deployItemName))
			fmt.Printf("  \n- file reference to deploy item definition added to blueprint")
			fmt.Printf("  \n- import definitions added to blueprint")
		},
	}

	opts.AddFlags(cmd.Flags())

	return cmd
}

func (o *addManifestDeployItemOptions) Complete(args []string) error {
	o.deployItemName = args[0]

	if err := o.parseParameterDefinitions(); err != nil {
		return err
	}

	return o.validate()
}

func (o *addManifestDeployItemOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.componentPath,
		"component-directory",
		".",
		"path to component directory (optional, default is current directory)")
	o.files = fs.StringArray(
		"manifest-file",
		[]string{},
		"manifest file containing one kubernetes resource")
	o.importParams = fs.StringArray(
		"import-param",
		[]string{},
		"import parameter as name:integer|string|boolean, e.g. replicas:integer")
	fs.StringVar(&o.updateStrategy,
		"update-strategy",
		"update",
		"update stategy")
	fs.StringVar(&o.policy,
		"policy",
		"manage",
		"policy")
	fs.StringVar(&o.clusterParam,
		"cluster-param",
		"targetCluster",
		"import parameter name for the target resource containing the access data of the target cluster")
}

func (o *addManifestDeployItemOptions) parseParameterDefinitions() (err error) {
	p := components.ParameterDefinitionParser{}

	o.importDefinitions, err = p.ParseImportDefinitions(o.importParams)
	if err != nil {
		return err
	}

	o.replacement = map[string]string{}
	for paramName := range o.importDefinitions {
		o.replacement[paramName] = string(uuid.NewUUID())
	}

	return nil
}

func (o *addManifestDeployItemOptions) validate() error {
	if !identityKeyValidationRegexp.Match([]byte(o.deployItemName)) {
		return fmt.Errorf("the deploy item name must consist of lower case alphanumeric characters, '-', '_' " +
			"or '+', and must start and end with an alphanumeric character")
	}

	if o.clusterParam == "" {
		return fmt.Errorf("cluster-param is missing")
	}

	if o.files == nil || len(*(o.files)) == 0 {
		return fmt.Errorf("no manifest files specified")
	}

	for _, path := range *(o.files) {
		fileInfo, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("manifest file %s does not exist", path)
			}
			return err
		}
		if fileInfo.IsDir() {
			return fmt.Errorf("manifest file %s is a directory", path)
		}
	}

	err := o.checkIfDeployItemNotAlreadyAdded()
	if err != nil {
		return err
	}

	return nil
}

func (o *addManifestDeployItemOptions) run(ctx context.Context, log logr.Logger) error {
	err := o.createExecutionFile()
	if err != nil {
		return err
	}

	blueprintPath := util.BlueprintDirectoryPath(o.componentPath)
	blueprint, err := blueprints.NewBlueprintReader(blueprintPath).Read()
	if err != nil {
		return err
	}

	blueprintBuilder := blueprints.NewBlueprintBuilder(blueprint)

	if blueprintBuilder.ExistsDeployExecution(o.deployItemName) {
		return fmt.Errorf("The blueprint already contains a deploy item %s\n", o.deployItemName)
	}

	blueprintBuilder.AddDeployExecution(o.deployItemName)
	blueprintBuilder.AddImportForTarget(o.clusterParam)
	blueprintBuilder.AddImportsFromMap(o.importDefinitions)

	return blueprints.NewBlueprintWriter(blueprintPath).Write(blueprint)
}

func (o *addManifestDeployItemOptions) checkIfDeployItemNotAlreadyAdded() error {
	_, err := os.Stat(util.ExecutionFilePath(o.componentPath, o.deployItemName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return fmt.Errorf("Deploy item was already added. The corresponding deploy execution file %s already exists\n",
		util.ExecutionFilePath(o.componentPath, o.deployItemName))
}

// parseImportDefinition creates a new ImportDefinition from a given parameter definition string.
// The parameter definition string must have the format "name:type", for example "replicas:integer".
// The supported types are: string, boolean, integer
func (o *addManifestDeployItemOptions) parseImportDefinition(paramDef string) (*v1alpha1.ImportDefinition, error) {
	p := components.ParameterDefinitionParser{}
	fieldValueDef, err := p.ParseFieldValueDefinition(paramDef)
	if err != nil {
		return nil, err
	}

	required := true

	return &v1alpha1.ImportDefinition{
		FieldValueDefinition: *fieldValueDef,
		Required:             &required,
	}, nil
}

func (o *addManifestDeployItemOptions) createExecutionFile() error {
	manifests, err := o.getManifests()
	if err != nil {
		return err
	}

	f, err := os.Create(util.ExecutionFilePath(o.componentPath, o.deployItemName))
	if err != nil {
		return err
	}

	defer f.Close()

	err = o.writeExecution(f)
	if err != nil {
		return err
	}

	_, err = f.WriteString(manifests)

	return err
}

const manifestExecutionTemplate = `deployItems:
- name: {{.DeployItemName}}
  type: landscaper.gardener.cloud/kubernetes-manifest
  target:
    name: {{.TargetNameExpression}}
    namespace: {{.TargetNamespaceExpression}}
  config:
    apiVersion: manifest.deployer.landscaper.gardener.cloud/v1alpha2
    kind: ProviderConfiguration
    updateStrategy: {{.UpdateStrategy}}
`

func (o *addManifestDeployItemOptions) writeExecution(f io.Writer) error {
	t, err := template.New("").Parse(manifestExecutionTemplate)
	if err != nil {
		return err
	}

	data := struct {
		DeployItemName            string
		TargetNameExpression      string
		TargetNamespaceExpression string
		UpdateStrategy            string
	}{
		DeployItemName:            o.deployItemName,
		TargetNameExpression:      blueprints.GetTargetNameExpression(o.clusterParam),
		TargetNamespaceExpression: blueprints.GetTargetNamespaceExpression(o.clusterParam),
		UpdateStrategy:            o.updateStrategy,
	}

	err = t.Execute(f, data)
	if err != nil {
		return err
	}

	return nil
}

func (o *addManifestDeployItemOptions) getManifests() (string, error) {
	data, err := o.getManifestsYaml()
	if err != nil {
		return "", err
	}

	stringData := string(data)
	stringData = indentLines(stringData, 4)
	return stringData, nil
}

func indentLines(data string, n int) string {
	indent := strings.Repeat(" ", n)
	return indent + strings.ReplaceAll(data, "\n", "\n"+indent)
}

func (o *addManifestDeployItemOptions) getManifestsYaml() ([]byte, error) {
	manifests, err := o.readManifests()
	if err != nil {
		return nil, err
	}

	m := map[string][]managedresource.Manifest{
		"manifests": manifests,
	}

	data, err := yaml.Marshal(m)
	if err != nil {
		return nil, err
	}

	data = o.replaceUUIDsByImportTemplates(data)

	return data, nil
}

func (o *addManifestDeployItemOptions) readManifests() ([]managedresource.Manifest, error) {
	manifests := []managedresource.Manifest{}

	if o.files == nil {
		return manifests, nil
	}

	for _, filename := range *o.files {
		m, err := o.readManifest(filename)
		if err != nil {
			return manifests, err
		}

		manifests = append(manifests, *m)
	}

	return manifests, nil
}

func (o *addManifestDeployItemOptions) readManifest(filename string) (*managedresource.Manifest, error) {
	yamlData, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var m interface{}
	err = yaml.Unmarshal(yamlData, &m)
	if err != nil {
		return nil, err
	}

	m = o.replaceParamsByUUIDs(m)

	// render to string
	uuidData, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	m2 := &managedresource.Manifest{
		Policy: managedresource.ManifestPolicy(o.policy),
		Manifest: &runtime.RawExtension{
			Raw: uuidData,
		},
	}

	return m2, nil
}

func (o *addManifestDeployItemOptions) replaceParamsByUUIDs(in interface{}) interface{} {
	switch m := in.(type) {
	case map[string]interface{}:
		for k := range m {
			m[k] = o.replaceParamsByUUIDs(m[k])
		}
		return m

	case []interface{}:
		for k := range m {
			m[k] = o.replaceParamsByUUIDs(m[k])
		}
		return m

	case string:
		newValue, ok := o.replacement[m]
		if ok {
			return newValue
		}
		return m

	default:
		return m
	}
}

func (o *addManifestDeployItemOptions) replaceUUIDsByImportTemplates(data []byte) []byte {
	s := string(data)

	for paramName, uuid := range o.replacement {
		newValue := blueprints.GetImportExpression(paramName)
		s = strings.ReplaceAll(s, uuid, newValue)
	}

	return []byte(s)
}
