package dependencybuild

import (
	_ "embed"
	v1alpha12 "github.com/redhat-appstudio/jvm-build-service/pkg/apis/jvmbuildservice/v1alpha1"
	"github.com/redhat-appstudio/jvm-build-service/pkg/reconciler/artifactbuild"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"strconv"
	"strings"
)

const (
	WorkspaceBuildSettings = "build-settings"
	WorkspaceSource        = "source"
)

//go:embed scripts/maven-settings.sh
var mavenSettings string

//go:embed scripts/gradle-settings.sh
var gradleSettings string

//go:embed scripts/maven-build.sh
var mavenBuild string

//go:embed scripts/gradle-build.sh
var gradleBuild string

func createPipelineSpec(maven bool, commitTime int64, userConfig *v1alpha12.UserConfig, preBuildString string, systemConfig *v1alpha12.SystemConfig) (*pipelinev1beta1.PipelineSpec, error) {
	var settings string
	var build string
	trueBool := true
	if maven {
		settings = mavenSettings
		build = mavenBuild
	} else {
		settings = gradleSettings
		build = gradleBuild
	}
	zero := int64(0)
	deployArgs := []string{
		"deploy-container",
		"--tar-path=$(workspaces.source.path)/hacbs-jvm-deployment-repo.tar.gz",
		"--task-run=$(context.taskRun.name)",
	}
	if userConfig.Spec.ImageRegistry.Host != "" {
		deployArgs = append(deployArgs, "--registry-host="+userConfig.Spec.ImageRegistry.Host)
	}
	if userConfig.Spec.ImageRegistry.Port != "" {
		deployArgs = append(deployArgs, "--registry-port="+userConfig.Spec.ImageRegistry.Port)
	}
	if userConfig.Spec.ImageRegistry.Owner != "" {
		deployArgs = append(deployArgs, "--registry-owner="+userConfig.Spec.ImageRegistry.Owner)
	}
	if userConfig.Spec.ImageRegistry.Repository != "" {
		deployArgs = append(deployArgs, "--registry-repository="+userConfig.Spec.ImageRegistry.Repository)
	}
	if userConfig.Spec.ImageRegistry.Insecure {
		deployArgs = append(deployArgs, "--registry-insecure=")
	}
	if userConfig.Spec.ImageRegistry.PrependTag != "" {
		deployArgs = append(deployArgs, "--registry-prepend-tag="+userConfig.Spec.ImageRegistry.PrependTag)
	}

	preprocessorArgs := []string{
		"maven-prepare",
		"-r",
		"$(params.CACHE_URL)",
		"$(workspaces." + WorkspaceSource + ".path)",
	}
	if !maven {
		preprocessorArgs[0] = "gradle-prepare"
	}
	defaultContainerRequestMemory, err := resource.ParseQuantity(settingOrDefault(userConfig.Spec.BuildSettings.TaskRequestMemory, "256Mi"))
	if err != nil {
		return nil, err
	}
	buildContainerRequestMemory, err := resource.ParseQuantity(settingOrDefault(userConfig.Spec.BuildSettings.BuildRequestMemory, "512Mi"))
	if err != nil {
		return nil, err
	}
	defaultContainerRequestCPU, err := resource.ParseQuantity(settingOrDefault(userConfig.Spec.BuildSettings.TaskRequestCPU, "10m"))
	if err != nil {
		return nil, err
	}
	defaultContainerLimitCPU, err := resource.ParseQuantity(settingOrDefault(userConfig.Spec.BuildSettings.TaskLimitCPU, "300m"))
	if err != nil {
		return nil, err
	}
	buildContainerRequestCPU, err := resource.ParseQuantity(settingOrDefault(userConfig.Spec.BuildSettings.BuildRequestCPU, "300m"))
	if err != nil {
		return nil, err
	}
	gitCloneImage := systemConfig.Spec.GitCloneImage
	if gitCloneImage == "" {
		gitCloneImage = "gcr.io/tekton-releases/github.com/tektoncd/pipeline/cmd/git-init:v0.21.0" //TODO: remove hard coded fallback
	}

	//we pass in the same params to every step
	commonParams := []v1alpha1.ParamSpec{
		{Name: PipelineBuildId, Type: pipelinev1beta1.ParamTypeString},
		{Name: PipelineScmUrl, Type: pipelinev1beta1.ParamTypeString},
		{Name: PipelineScmTag, Type: pipelinev1beta1.ParamTypeString},
		{Name: PipelineImage, Type: pipelinev1beta1.ParamTypeString},
		{Name: PipelineGoals, Type: pipelinev1beta1.ParamTypeArray},
		{Name: PipelineJavaVersion, Type: pipelinev1beta1.ParamTypeString},
		{Name: PipelineToolVersion, Type: pipelinev1beta1.ParamTypeString},
		{Name: PipelinePath, Type: pipelinev1beta1.ParamTypeString},
		{Name: PipelineEnforceVersion, Type: pipelinev1beta1.ParamTypeString},
		{Name: PipelineRequestProcessorImage, Type: pipelinev1beta1.ParamTypeString},
		{Name: PipelineIgnoredArtifacts, Type: pipelinev1beta1.ParamTypeString},
		{Name: PipelineCacheUrl, Type: pipelinev1beta1.ParamTypeString, Default: &pipelinev1beta1.ArrayOrString{Type: pipelinev1beta1.ParamTypeString, StringVal: "http://jvm-build-workspace-artifact-cache.$(context.pipelineRun.namespace).svc.cluster.local/v1/cache/default/" + strconv.FormatInt(commitTime, 10)}},
	}

	if !maven {
		commonParams = append(commonParams, v1alpha1.ParamSpec{Name: PipelineGradleManipulatorArgs, Type: pipelinev1beta1.ParamTypeString, Default: &pipelinev1beta1.ArrayOrString{Type: pipelinev1beta1.ParamTypeString, StringVal: "-DdependencySource=NONE -DignoreUnresolvableDependencies=true -DpluginRemoval=ALL -DversionModification=false"}})
	}
	cloneTask := pipelinev1beta1.TaskSpec{
		Workspaces: []pipelinev1beta1.WorkspaceDeclaration{{Name: WorkspaceBuildSettings}, {Name: WorkspaceSource}},
		Params:     commonParams,
		Steps: []pipelinev1beta1.Step{
			{
				Container: v1.Container{
					Name:  "git-clone",
					Image: gitCloneImage,
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{"memory": defaultContainerRequestMemory, "cpu": defaultContainerRequestCPU},
						Limits:   v1.ResourceList{"memory": defaultContainerRequestMemory, "cpu": defaultContainerLimitCPU},
					},
					Args: []string{"-path=$(workspaces." + WorkspaceSource + ".path)", "-url=$(params." + PipelineScmUrl + ")", "-revision=$(params." + PipelineScmTag + ")"},
				},
			},
		},
	}
	buildSetup := pipelinev1beta1.TaskSpec{
		Workspaces: []pipelinev1beta1.WorkspaceDeclaration{{Name: WorkspaceBuildSettings}, {Name: WorkspaceSource}},
		Params:     commonParams,
		Results:    []pipelinev1beta1.TaskResult{{Name: artifactbuild.Contaminants}, {Name: artifactbuild.DeployedResources}},
		Steps: []pipelinev1beta1.Step{
			{
				Container: v1.Container{
					Name:            "settings",
					Image:           "registry.access.redhat.com/ubi8/ubi:8.5", //TODO: should not be hard coded
					SecurityContext: &v1.SecurityContext{RunAsUser: &zero},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{"memory": defaultContainerRequestMemory, "cpu": defaultContainerRequestCPU},
						Limits:   v1.ResourceList{"memory": defaultContainerRequestMemory, "cpu": defaultContainerLimitCPU},
					},
				},
				Script: settings,
			},
			{
				Container: v1.Container{
					Name:            "preprocessor",
					Image:           "$(params." + PipelineRequestProcessorImage + ")",
					SecurityContext: &v1.SecurityContext{RunAsUser: &zero},
					Resources: v1.ResourceRequirements{
						//TODO: make configurable
						Requests: v1.ResourceList{"memory": defaultContainerRequestMemory, "cpu": defaultContainerRequestCPU},
						Limits:   v1.ResourceList{"memory": defaultContainerRequestMemory, "cpu": defaultContainerLimitCPU},
					},
					Args: preprocessorArgs,
				},
			},
			{
				Container: v1.Container{
					Name:            "build",
					Image:           "$(params." + PipelineImage + ")",
					WorkingDir:      "$(workspaces." + WorkspaceSource + ".path)/$(params." + PipelinePath + ")",
					SecurityContext: &v1.SecurityContext{RunAsUser: &zero},
					Env: []v1.EnvVar{
						{Name: PipelineCacheUrl, Value: "$(params." + PipelineCacheUrl + ")"},
						{Name: PipelineEnforceVersion, Value: "$(params." + PipelineEnforceVersion + ")"},
					},
					Resources: v1.ResourceRequirements{
						//TODO: limits management and configuration
						Requests: v1.ResourceList{"memory": buildContainerRequestMemory, "cpu": buildContainerRequestCPU},
					},
					Args: []string{"$(params.GOALS[*])"},
				},
				Script: strings.ReplaceAll(build, "{{PRE_BUILD_SCRIPT}}", preBuildString),
			},
			{
				Container: v1.Container{
					Name:            "deploy-and-check-for-contaminates",
					Image:           "$(params." + PipelineRequestProcessorImage + ")",
					SecurityContext: &v1.SecurityContext{RunAsUser: &zero},
					Env: []v1.EnvVar{
						{Name: "REGISTRY_TOKEN", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: v1alpha12.UserSecretName}, Key: v1alpha12.UserSecretTokenKey, Optional: &trueBool}}},
					},
					Resources: v1.ResourceRequirements{
						//TODO: make configurable
						Requests: v1.ResourceList{"memory": buildContainerRequestMemory, "cpu": defaultContainerRequestCPU},
						Limits:   v1.ResourceList{"memory": buildContainerRequestMemory, "cpu": defaultContainerLimitCPU},
					},
					Args: deployArgs,
				},
			},
		},
	}

	taskList := []pipelinev1beta1.PipelineTask{
		{
			Name: "hacbs-git-clone-step",
			TaskSpec: &pipelinev1beta1.EmbeddedTask{
				TaskSpec: cloneTask,
			},
			Params: []pipelinev1beta1.Param{}, Workspaces: []pipelinev1beta1.WorkspacePipelineTaskBinding{
			{Name: WorkspaceBuildSettings, Workspace: WorkspaceBuildSettings},
			{Name: WorkspaceSource, Workspace: "source"},
		},
		},
	}
	// pre build tasks should handle things like source code scanning
	for _, i := range systemConfig.Spec.PreBuildTasks {
		taskList = append(taskList, pipelinev1beta1.PipelineTask{
			Name:       i.Name,
			TaskRef:    &pipelinev1beta1.TaskRef{Name: i.TaskRef.Name, Bundle: i.TaskRef.Bundle},
			Params:     i.Params,
			Workspaces: i.Workspaces,
		})
	}
	taskList = append(taskList, pipelinev1beta1.PipelineTask{
		Name: artifactbuild.TaskName,
		TaskSpec: &pipelinev1beta1.EmbeddedTask{
			TaskSpec: buildSetup,
		},
		Params: []pipelinev1beta1.Param{},
		Workspaces: []pipelinev1beta1.WorkspacePipelineTaskBinding{
			{Name: WorkspaceBuildSettings, Workspace: WorkspaceBuildSettings},
			{Name: WorkspaceSource, Workspace: "source"},
		}})
	for _, i := range systemConfig.Spec.PostBuildTasks {
		taskList = append(taskList, pipelinev1beta1.PipelineTask{
			Name:       i.Name,
			TaskRef:    &pipelinev1beta1.TaskRef{Name: i.TaskRef.Name, Bundle: i.TaskRef.Bundle},
			Params:     i.Params,
			Workspaces: i.Workspaces,
		})
	}
	ps := &pipelinev1beta1.PipelineSpec{
		Tasks:      taskList,
		Workspaces: []v1alpha1.PipelineWorkspaceDeclaration{{Name: WorkspaceBuildSettings}, {Name: WorkspaceSource}},
	}

	for _, i := range buildSetup.Results {
		ps.Results = append(ps.Results, pipelinev1beta1.PipelineResult{Name: i.Name, Description: i.Description, Value: "$(tasks." + artifactbuild.TaskName + ".results." + i.Name + ")"})
	}
	for _, i := range buildSetup.Params {
		ps.Params = append(ps.Params, pipelinev1beta1.ParamSpec{Name: i.Name, Description: i.Description, Default: i.Default, Type: i.Type})
		var value pipelinev1beta1.ArrayOrString
		if i.Type == pipelinev1beta1.ParamTypeString {
			value = pipelinev1beta1.ArrayOrString{Type: i.Type, StringVal: "$(params." + i.Name + ")"}
		} else {
			value = pipelinev1beta1.ArrayOrString{Type: i.Type, ArrayVal: []string{"$(params." + i.Name + "[*])"}}
		}
		for j := range ps.Tasks {
			if ps.Tasks[j].TaskSpec != nil {
				ps.Tasks[j].Params = append(ps.Tasks[j].Params, pipelinev1beta1.Param{
					Name:  i.Name,
					Value: value})
			}
		}
	}
	return ps, nil
}

func settingOrDefault(setting, def string) string {
	if len(strings.TrimSpace(setting)) == 0 {
		return def
	}
	return setting
}
