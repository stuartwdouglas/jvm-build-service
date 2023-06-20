package dependencybuild

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	v1alpha12 "github.com/redhat-appstudio/jvm-build-service/pkg/apis/jvmbuildservice/v1alpha1"
	"github.com/redhat-appstudio/jvm-build-service/pkg/reconciler/artifactbuild"
	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	WorkspaceBuildSettings = "build-settings"
	WorkspaceSource        = "source"
	WorkspaceTls           = "tls"
)

//go:embed scripts/maven-settings.sh
var mavenSettings string

//go:embed scripts/gradle-settings.sh
var gradleSettings string

//go:embed scripts/maven-build.sh
var mavenBuild string

//go:embed scripts/gradle-build.sh
var gradleBuild string

//go:embed scripts/sbt-build.sh
var sbtBuild string

//go:embed scripts/ant-build.sh
var antBuild string

//go:embed scripts/install-package.sh
var packageTemplate string

//go:embed scripts/entry-script.sh
var entryScript string

func createPipelineSpec(tool string, commitTime int64, jbsConfig *v1alpha12.JBSConfig, systemConfig *v1alpha12.SystemConfig, recipe *v1alpha12.BuildRecipe, db *v1alpha12.DependencyBuild, paramValues []pipelinev1beta1.Param, buildRequestProcessorImage string) (*pipelinev1beta1.PipelineSpec, string, error) {

	zero := int64(0)
	verifyBuiltArtifactsArgs := []string{
		"verify-built-artifacts",
		"--repository-url=$(params.CACHE_URL)?upstream-only=true",
		"--global-settings=/usr/share/maven/conf/settings.xml",
		"--settings=$(workspaces.build-settings.path)/settings.xml",
		"--deploy-path=$(workspaces.source.path)/artifacts",
		"--task-run-name=$(context.taskRun.name)",
		"--results-file=$(results." + artifactbuild.PipelineResultPassedVerification + ".path)",
	}

	if !jbsConfig.Spec.RequireArtifactVerification {
		verifyBuiltArtifactsArgs = append(verifyBuiltArtifactsArgs, "--report-only")
	}

	if len(recipe.AllowedDifferences) > 0 {
		for _, i := range recipe.AllowedDifferences {
			verifyBuiltArtifactsArgs = append(verifyBuiltArtifactsArgs, "--excludes="+i)
		}
	}

	deployArgs := []string{
		"deploy-container",
		"--path=$(workspaces.source.path)/artifacts",
		"--logs-path=$(workspaces.source.path)/logs",
		"--build-info-path=$(workspaces.source.path)/build-info",
		"--source-path=$(workspaces.source.path)/source",
		"--task-run=$(context.taskRun.name)",
		"--scm-uri=" + db.Spec.ScmInfo.SCMURL,
		"--scm-commit=" + db.Spec.ScmInfo.CommitHash,
	}
	imageRegistry := jbsConfig.ImageRegistry()
	if imageRegistry.Host != "" {
		deployArgs = append(deployArgs, "--registry-host="+imageRegistry.Host)
	}
	if imageRegistry.Port != "" {
		deployArgs = append(deployArgs, "--registry-port="+imageRegistry.Port)
	}
	if imageRegistry.Owner != "" {
		deployArgs = append(deployArgs, "--registry-owner="+imageRegistry.Owner)
	}
	if imageRegistry.Repository != "" {
		deployArgs = append(deployArgs, "--registry-repository="+imageRegistry.Repository)
	}
	if imageRegistry.Insecure {
		deployArgs = append(deployArgs, "--registry-insecure")
	}
	if jbsConfig.ImageRegistry().PrependTag != "" {
		deployArgs = append(deployArgs, "--registry-prepend-tag="+imageRegistry.PrependTag)
	}

	install := ""
	for count, i := range recipe.AdditionalDownloads {
		if i.FileType == "tar" {
			if i.BinaryPath == "" {
				install = "echo 'Binary path not specified for package " + i.Uri + "'; exit 1"
			}

		} else if i.FileType == "executable" {
			if i.FileName == "" {
				install = "echo 'File name not specified for package " + i.Uri + "'; exit 1"
			}
		} else if i.FileType == "rpm" {
			if i.PackageName == "" {
				install = "echo 'Package name not specified for rpm type'; exit 1"
			}
		} else {
			//unknown
			//we still run the pipeline so there is logs
			install = "echo 'Unknown file type " + i.FileType + " for package " + i.Uri + "'; exit 1"
			break
		}
		template := packageTemplate
		fileName := i.FileName
		if fileName == "" {
			fileName = "package-" + strconv.Itoa(count)
		}
		template = strings.ReplaceAll(template, "{URI}", i.Uri)
		template = strings.ReplaceAll(template, "{FILENAME}", fileName)
		template = strings.ReplaceAll(template, "{SHA256}", i.Sha256)
		template = strings.ReplaceAll(template, "{TYPE}", i.FileType)
		template = strings.ReplaceAll(template, "{BINARY_PATH}", i.BinaryPath)
		template = strings.ReplaceAll(template, "{PACKAGE_NAME}", i.PackageName)
		install = install + template
	}

	preprocessorArgs := []string{
		"maven-prepare",
		"-r",
		"$(params.CACHE_URL)",
		"$(workspaces." + WorkspaceSource + ".path)/workspace",
	}
	additionalMemory := recipe.AdditionalMemory
	if systemConfig.Spec.MaxAdditionalMemory > 0 && additionalMemory > systemConfig.Spec.MaxAdditionalMemory {
		additionalMemory = systemConfig.Spec.MaxAdditionalMemory
	}
	var settings string
	var build string
	trueBool := true
	if tool == "maven" {
		settings = mavenSettings
		build = mavenBuild
	} else if tool == "gradle" {
		settings = gradleSettings
		build = gradleBuild
		preprocessorArgs[0] = "gradle-prepare"
	} else if tool == "sbt" {
		settings = "" //TODO: look at removing the setttings step altogether
		build = sbtBuild
		preprocessorArgs[0] = "sbt-prepare"
	} else if tool == "ant" {
		settings = mavenSettings
		build = antBuild
		preprocessorArgs[0] = "ant-prepare"
	} else {
		settings = "echo unknown build tool " + tool + " && exit 1"
		build = ""
	}
	//horrible hack
	//we need to get our TLS CA's into our trust store
	//we just add it at the start of the build
	build = artifactbuild.InstallKeystoreScript() + "\n" + build
	gitArgs := ""
	if db.Spec.ScmInfo.Private {
		gitArgs = "echo \"$GIT_TOKEN\"  > $HOME/.git-credentials\nchmod 400 $HOME/.git-credentials\n"
		gitArgs = gitArgs + "echo '[credential]\n        helper=store\n' > $HOME/.gitconfig\n"
	}
	gitArgs = gitArgs + "git clone $(params." + PipelineParamScmUrl + ") $(workspaces." + WorkspaceSource + ".path)/workspace && cd $(workspaces." + WorkspaceSource + ".path)/workspace && git reset --hard $(params." + PipelineParamScmHash + ")"

	if !recipe.DisableSubmodules {
		gitArgs = gitArgs + " && git submodule init && git submodule update --recursive"
	}
	defaultContainerRequestMemory, err := resource.ParseQuantity(settingOrDefault(jbsConfig.Spec.BuildSettings.TaskRequestMemory, "512Mi"))
	if err != nil {
		return nil, "", err
	}
	defaultBuildContainerRequestMemory, err := resource.ParseQuantity(settingOrDefault(jbsConfig.Spec.BuildSettings.BuildRequestMemory, "1024Mi"))
	if err != nil {
		return nil, "", err
	}
	defaultContainerRequestCPU, err := resource.ParseQuantity(settingOrDefault(jbsConfig.Spec.BuildSettings.TaskRequestCPU, "10m"))
	if err != nil {
		return nil, "", err
	}
	defaultContainerLimitCPU, err := resource.ParseQuantity(settingOrDefault(jbsConfig.Spec.BuildSettings.TaskLimitCPU, "300m"))
	if err != nil {
		return nil, "", err
	}
	buildah := `

       chown root:root /var/lib/containers

      sed -i 's/^\s*short-name-mode\s*=\s*.*/short-name-mode = "disabled"/' /etc/containers/registries.conf

      # Setting new namespace to run buildah - 2^32-2
      echo 'root:1:4294967294' | tee -a /etc/subuid >> /etc/subgid

      unshare -Uf --keep-caps -r --map-users 1,1,65536 --map-groups 1,1,65536  -- buildah build --storage-driver=vfs \
        --no-cache \
        --ulimit nofile=4096:4096 \
        -f "Dockerfile" -t test .`
	buildContainerRequestMemory := defaultBuildContainerRequestMemory
	if additionalMemory > 0 {
		additional := resource.MustParse(fmt.Sprintf("%dMi", additionalMemory))
		buildContainerRequestMemory.Add(additional)
		defaultContainerRequestMemory.Add(additional)
	}
	buildRepos := ""
	if len(recipe.Repositories) > 0 {
		for c, i := range recipe.Repositories {
			if c == 0 {
				buildRepos = "-" + i
			} else {
				buildRepos = buildRepos + "," + i
			}
		}
	}
	build = strings.ReplaceAll(build, "{{INSTALL_PACKAGE_SCRIPT}}", install)
	build = strings.ReplaceAll(build, "{{PRE_BUILD_SCRIPT}}", recipe.PreBuildScript)
	build = strings.ReplaceAll(build, "{{POST_BUILD_SCRIPT}}", recipe.PostBuildScript)
	cacheUrl := "https://jvm-build-workspace-artifact-cache-tls." + jbsConfig.Namespace + ".svc.cluster.local/v2/cache/rebuild"
	if jbsConfig.Spec.CacheSettings.DisableTLS {
		cacheUrl = "http://jvm-build-workspace-artifact-cache." + jbsConfig.Namespace + ".svc.cluster.local/v2/cache/rebuild"
	}

	buildSetup := pipelinev1beta1.TaskSpec{
		Workspaces: []pipelinev1beta1.WorkspaceDeclaration{{Name: WorkspaceBuildSettings}, {Name: WorkspaceSource}, {Name: WorkspaceTls}},
		Params: []pipelinev1beta1.ParamSpec{
			{Name: PipelineBuildId, Type: pipelinev1beta1.ParamTypeString},

			{Name: PipelineParamScmUrl, Type: pipelinev1beta1.ParamTypeString},
			{Name: PipelineParamScmTag, Type: pipelinev1beta1.ParamTypeString},
			{Name: PipelineParamScmHash, Type: pipelinev1beta1.ParamTypeString},
			{Name: PipelineParamChainsGitUrl, Type: pipelinev1beta1.ParamTypeString},
			{Name: PipelineParamChainsGitCommit, Type: pipelinev1beta1.ParamTypeString},
			{Name: PipelineParamImage, Type: pipelinev1beta1.ParamTypeString},
			{Name: PipelineParamGoals, Type: pipelinev1beta1.ParamTypeArray},
			{Name: PipelineParamJavaVersion, Type: pipelinev1beta1.ParamTypeString},
			{Name: PipelineParamToolVersion, Type: pipelinev1beta1.ParamTypeString},
			{Name: PipelineParamPath, Type: pipelinev1beta1.ParamTypeString},
			{Name: PipelineParamEnforceVersion, Type: pipelinev1beta1.ParamTypeString},
			{Name: PipelineParamRequestProcessorImage, Type: pipelinev1beta1.ParamTypeString},
			{Name: PipelineParamCacheUrl, Type: pipelinev1beta1.ParamTypeString, Default: &pipelinev1beta1.ArrayOrString{Type: pipelinev1beta1.ParamTypeString, StringVal: cacheUrl + buildRepos + "/" + strconv.FormatInt(commitTime, 10)}},
		},
		Results: []pipelinev1beta1.TaskResult{
			{Name: artifactbuild.PipelineResultContaminants},
			{Name: artifactbuild.PipelineResultDeployedResources},
			{Name: PipelineResultImage},
			{Name: PipelineResultImageDigest},
			{Name: artifactbuild.PipelineResultPassedVerification},
			{Name: artifactbuild.PipelineResultVerificationResult},
		},
		Steps: []pipelinev1beta1.Step{
			{
				Name:            "git-clone-and-settings",
				Image:           "quay.io/redhat-appstudio/buildah:v1.28",
				SecurityContext: &v1.SecurityContext{RunAsUser: &zero, Capabilities: &v1.Capabilities{Add: []v1.Capability{"SETFCAP"}}},
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{"memory": defaultContainerRequestMemory, "cpu": defaultContainerRequestCPU},
					Limits:   v1.ResourceList{"memory": defaultContainerRequestMemory, "cpu": defaultContainerLimitCPU},
				},
				WorkingDir: "$(workspaces.source.path)",
				Args:       []string{"$(params.GOALS[*])"},
				Script: createDockerFileComponents(gitArgs+"\n"+settings, "git", "", "$(params."+PipelineParamImage+")", true) + "\n" +
					createDockerFileComponents(artifactbuild.InstallKeystoreIntoBuildRequestProcessor(preprocessorArgs), "req-processor", "git", "$(params."+PipelineParamRequestProcessorImage+")", false) + "\n" +
					createDockerFileComponents(build, "build", "req-processor", "$(params."+PipelineParamImage+")", true) + "\n" +
					createDockerFileComponents(artifactbuild.InstallKeystoreIntoBuildRequestProcessor(verifyBuiltArtifactsArgs, deployArgs), "deploy", "build", "$(params."+PipelineParamRequestProcessorImage+")", false) + "\n" +
					"\n" + buildah + "\n",

				Env: []v1.EnvVar{
					{Name: PipelineParamCacheUrl, Value: "$(params." + PipelineParamCacheUrl + ")"},
					{Name: "GIT_TOKEN", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: v1alpha12.GitSecretName}, Key: v1alpha12.GitSecretTokenKey, Optional: &trueBool}}},
				},
			},
		},
	}

	ps := &pipelinev1beta1.PipelineSpec{
		Tasks: []pipelinev1beta1.PipelineTask{
			{
				Name: artifactbuild.TaskName,
				TaskSpec: &pipelinev1beta1.EmbeddedTask{
					TaskSpec: buildSetup,
				},
				Params: []pipelinev1beta1.Param{}, Workspaces: []pipelinev1beta1.WorkspacePipelineTaskBinding{
					{Name: WorkspaceBuildSettings, Workspace: WorkspaceBuildSettings},
					{Name: WorkspaceSource, Workspace: WorkspaceSource},
					{Name: WorkspaceTls, Workspace: WorkspaceTls},
				},
			},
		},
		Workspaces: []pipelinev1beta1.PipelineWorkspaceDeclaration{{Name: WorkspaceBuildSettings}, {Name: WorkspaceSource}, {Name: WorkspaceTls}},
	}

	for _, i := range buildSetup.Results {
		ps.Results = append(ps.Results, pipelinev1beta1.PipelineResult{Name: i.Name, Description: i.Description, Value: pipelinev1beta1.ResultValue{Type: pipelinev1beta1.ParamTypeString, StringVal: "$(tasks." + artifactbuild.TaskName + ".results." + i.Name + ")"}})
	}
	for _, i := range buildSetup.Params {
		ps.Params = append(ps.Params, pipelinev1beta1.ParamSpec{Name: i.Name, Description: i.Description, Default: i.Default, Type: i.Type})
		var value pipelinev1beta1.ArrayOrString
		if i.Type == pipelinev1beta1.ParamTypeString {
			value = pipelinev1beta1.ArrayOrString{Type: i.Type, StringVal: "$(params." + i.Name + ")"}
		} else {
			value = pipelinev1beta1.ArrayOrString{Type: i.Type, ArrayVal: []string{"$(params." + i.Name + "[*])"}}
		}
		ps.Tasks[0].Params = append(ps.Tasks[0].Params, pipelinev1beta1.Param{
			Name:  i.Name,
			Value: value})
	}

	//we generate a docker file that can be used to reproduce this build
	//this is for diagnostic purposes, if you have a failing build it can be really hard to figure out how to fix it without this
	df := "FROM " + extractParam(PipelineParamRequestProcessorImage, paramValues) + " AS build-request-processor" +
		"\nFROM " + strings.ReplaceAll(extractParam(PipelineParamRequestProcessorImage, paramValues), "hacbs-jvm-build-request-processor", "hacbs-jvm-cache") + " AS cache" +
		"\nFROM " + extractParam(PipelineParamImage, paramValues) +
		"\nUSER 0" +
		"\nWORKDIR /root" +
		"\nENV CACHE_URL=" + doSubstitution("$(params."+PipelineParamCacheUrl+")", paramValues, commitTime, buildRepos) +
		"\nENV BUILD_POLICY_DEFAULT_STORE_LIST=central,redhat,jboss,gradleplugins,confluent,gradle,eclipselink,jitpack,jsweet,jenkins,spring-plugins,dokkadev,ajoberstar,googleandroid,kotlinnative14linux,jcs,kotlin-bootstrap,kotlin-kotlin-dependencies" +
		"\nRUN mkdir -p /root/project /root/software/settings && microdnf install vim curl procps-ng bash-completion" +
		"\nCOPY --from=build-request-processor /deployments/ /root/software/build-request-processor" +
		// Copying JDK17 for the cache.
		"\nCOPY --from=build-request-processor /lib/jvm/jre-17 /root/software/system-java" +
		"\nCOPY --from=build-request-processor /etc/java/java-17-openjdk /etc/java/java-17-openjdk" +
		"\nCOPY --from=cache /deployments/ /root/software/cache" +
		"\nRUN " + doSubstitution(gitArgs, paramValues, commitTime, buildRepos) +
		"\nRUN echo " + base64.StdEncoding.EncodeToString([]byte("#!/bin/sh\n/root/software/system-java/bin/java -Dkube.disabled=true -Dquarkus.kubernetes-client.trust-certs=true -jar /root/software/cache/quarkus-run.jar >/root/cache.log &"+
		"\necho \"Please wait a few seconds for cache to start. Run 'tail -f cache.log'\"\n")) + " | base64 -d >/root/start-cache.sh" +
		"\nRUN echo " + base64.StdEncoding.EncodeToString([]byte(doSubstitution(settings, paramValues, commitTime, buildRepos))) + " | base64 -d >/root/settings.sh" +
		"\nRUN echo " + base64.StdEncoding.EncodeToString([]byte("#!/bin/sh\n/root/software/system-java/bin/java -jar /root/software/build-request-processor/quarkus-run.jar "+doSubstitution(strings.Join(preprocessorArgs, " "), paramValues, commitTime, buildRepos)+"\n")) + " | base64 -d >/root/preprocessor.sh" +
		"\nRUN echo " + base64.StdEncoding.EncodeToString([]byte(doSubstitution(build, paramValues, commitTime, buildRepos))) + " | base64 -d >/root/build.sh" +
		"\nRUN echo " + base64.StdEncoding.EncodeToString([]byte("#!/bin/sh\n/root/settings.sh\n/root/preprocessor.sh\ncd /root/project/workspace\n/root/build.sh "+strings.Join(extractArrayParam(PipelineParamGoals, paramValues), " ")+"\n")) + " | base64 -d >/root/run-full-build.sh" +
		"\nRUN echo " + base64.StdEncoding.EncodeToString([]byte(entryScript)) + " | base64 -d >/root/entry-script.sh" +
		"\nRUN chmod +x /root/*.sh" +
		"\nCMD [ \"/bin/bash\", \"/root/entry-script.sh\" ]"

	return ps, df, nil
}

func extractParam(key string, paramValues []pipelinev1beta1.Param) string {
	for _, i := range paramValues {
		if i.Name == key {
			return i.Value.StringVal
		}
	}
	return ""
}
func extractArrayParam(key string, paramValues []pipelinev1beta1.Param) []string {
	for _, i := range paramValues {
		if i.Name == key {
			return i.Value.ArrayVal
		}
	}
	return []string{}
}

func doSubstitution(script string, paramValues []pipelinev1beta1.Param, commitTime int64, buildRepos string) string {
	for _, i := range paramValues {
		if i.Value.Type == pipelinev1beta1.ParamTypeString {
			script = strings.ReplaceAll(script, "$(params."+i.Name+")", i.Value.StringVal)
		}
	}
	script = strings.ReplaceAll(script, "$(params.CACHE_URL)", "http://localhost:8080/v2/cache/rebuild"+buildRepos+"/"+strconv.FormatInt(commitTime, 10)+"/")
	script = strings.ReplaceAll(script, "$(workspaces.build-settings.path)", "/root/software/settings")
	script = strings.ReplaceAll(script, "$(workspaces.source.path)", "/root/project")
	script = strings.ReplaceAll(script, "$(workspaces.tls.path)", "/root/project/tls/service-ca.crt")
	return script
}

func settingOrDefault(setting, def string) string {
	if len(strings.TrimSpace(setting)) == 0 {
		return def
	}
	return setting
}

func createDockerFileComponents(script string, layername string, prevlayer string, image string, imageHasBase64 bool) string {
	//write the from section and copy what is required
	ret := fmt.Sprintf("cat >>Dockerfile <<RHTAPEOFMARKERFORHEREDOC\n"+
		"FROM %s as %s\n", image, layername)
	if prevlayer != "" {
		ret += fmt.Sprintf("COPY --from=%s  $(workspaces.source.path) $(workspaces.source.path)\n", prevlayer)
		ret += fmt.Sprintf("COPY --from=%s  $(workspaces.build-settings.path) $(workspaces.build-settings.path)\n", prevlayer)
		ret += fmt.Sprintf("COPY --from=%s  $(workspaces.tls.path) $(workspaces.tls.path)\n", prevlayer)
	} else {
		ret += fmt.Sprintf("COPY $(workspaces.source.path) $(workspaces.source.path)\n", prevlayer)
		ret += fmt.Sprintf("COPY $(workspaces.build-settings.path) $(workspaces.build-settings.path)\n", prevlayer)
		ret += fmt.Sprintf("COPY $(workspaces.tls.path) $(workspaces.tls.path)\n", prevlayer)
	}
	ret += "\nRHTAPEOFMARKERFORHEREDOC\n"
	//now write the script to the dockerfile
	//we can't just copy it, as the format does not allow that
	//we can't just base64 encode it in the golang, as that would break parameter substitution
	//instead we need to base64 it in the step itself
	ret += "echo -n 'RUN echo ' >>Dockerfile\n"
	if imageHasBase64 {
		ret += fmt.Sprintf("cat <<RHTAPEOFMARKERFORHEREDOC | base64 -w 0 >>Dockerfile \n")
		ret += script
		ret += "\nRHTAPEOFMARKERFORHEREDOC\n"
	} else {
		ret += "echo -n " + base64.StdEncoding.EncodeToString([]byte(script)) + "\n" //no param substitution in this case
	}
	ret += "echo ' | base64 -d >script.sh' >>Dockerfile\n"
	ret += "echo 'RUN chmod +x ./script.sh && ./script.sh' >>Dockerfile\n"
	return ret
}
