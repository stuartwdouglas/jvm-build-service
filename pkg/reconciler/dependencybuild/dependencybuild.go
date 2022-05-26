package dependencybuild

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"github.com/redhat-appstudio/jvm-build-service/pkg/reconciler/artifactbuildrequest"
	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/redhat-appstudio/jvm-build-service/pkg/apis/jvmbuildservice/v1alpha1"
)

const (
	//TODO eventually we'll need to decide if we want to make this tuneable
	contextTimeout   = 300 * time.Second
	PipelineRunLabel = "jvmbuildservice.io/dependencybuild-pipelinerun"
	PipelineScmUrl   = "url"
	PipelineScmTag   = "tag"
	PipelinePath     = "context"
	PipelineImage    = "image"
)

type ReconcileDependencyBuild struct {
	client        client.Client
	scheme        *runtime.Scheme
	eventRecorder record.EventRecorder
}

func newReconciler(mgr ctrl.Manager) reconcile.Reconciler {
	return &ReconcileDependencyBuild{
		client:        mgr.GetClient(),
		scheme:        mgr.GetScheme(),
		eventRecorder: mgr.GetEventRecorderFor("DependencyBuild"),
	}
}

func (r *ReconcileDependencyBuild) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	// Set the ctx to be Background, as the top-level context for incoming requests.
	ctx, cancel := context.WithTimeout(ctx, contextTimeout)
	defer cancel()

	db := v1alpha1.DependencyBuild{}
	err := r.client.Get(ctx, request.NamespacedName, &db)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	//we validate that our dep id hash is still valid
	//if a field has been modified we need to update the label
	//which may result in a new build
	depId := hashToString(db.Spec.ScmInfo.SCMURL + db.Spec.ScmInfo.Tag + db.Spec.ScmInfo.Path)
	if depId != db.Labels[artifactbuildrequest.DependencyBuildIdLabel] {
		//if our id has changed we just update the label and set our state back to new
		//this will kick off a new build
		db.Labels[artifactbuildrequest.DependencyBuildIdLabel] = depId
		db.Status.State = v1alpha1.DependencyBuildStateNew
		return reconcile.Result{}, r.client.Update(ctx, &db)
	}

	switch db.Status.State {
	case "", v1alpha1.DependencyBuildStateNew:
		return r.handleStateNew(ctx, &db)
	case v1alpha1.DependencyBuildStateDetect:
		return r.handleStateDetect(ctx, &db)
	case v1alpha1.DependencyBuildStateComplete, v1alpha1.DependencyBuildStateFailed:
		return reconcile.Result{}, nil
	case v1alpha1.DependencyBuildStateBuilding:
		return r.handleStateBuilding(ctx, depId, &db)
	case v1alpha1.DependencyBuildStateContaminated:
		return r.handleStateContaminated(ctx, &db)
	}

	return reconcile.Result{}, nil
}

func hashToString(unique string) string {
	hash := md5.Sum([]byte(unique))
	depId := hex.EncodeToString(hash[:])
	return depId
}

func (r *ReconcileDependencyBuild) handleStateNew(ctx context.Context, db *v1alpha1.DependencyBuild) (reconcile.Result, error) {
	//TODO: this is currently a huge hard coded hack
	//we hard code 3 potential build recipes (images)
	//then move the state to DependencyBuildStateDetect
	//once this is not longer a hard coded stub it should trigger a TR/PR
	//that looks at the repository and figures out which builder to use
	db.Status.PotentialBuildRecipes = []*v1alpha1.BuildRecipe{{Image: "quay.io/sdouglas/hacbs-jdk11-builder:latest"},
		{Image: "quay.io/sdouglas/hacbs-jdk8-builder:latest"},
		{Image: "quay.io/sdouglas/hacbs-jdk17-builder:latest"}}
	db.Status.State = v1alpha1.DependencyBuildStateDetect
	return reconcile.Result{}, r.client.Status().Update(ctx, db)
}

func (r *ReconcileDependencyBuild) handleStateDetect(ctx context.Context, db *v1alpha1.DependencyBuild) (reconcile.Result, error) {
	//TODO: read results of detect task

	return r.attemptNewRecipe(ctx, db)
}

func (r *ReconcileDependencyBuild) attemptNewRecipe(ctx context.Context, db *v1alpha1.DependencyBuild) (reconcile.Result, error) {
	//new build, kick off a pipeline run to run the build
	if db.Status.CurrentBuildRecipe != nil {
		db.Status.FailedBuildRecipes = append(db.Status.FailedBuildRecipes, db.Status.CurrentBuildRecipe)
	}
	if len(db.Status.PotentialBuildRecipes) == 0 {
		db.Status.State = v1alpha1.DependencyBuildStateFailed
		return reconcile.Result{}, r.client.Status().Update(ctx, db)
	}
	//pick the first recipe in the potential list
	db.Status.CurrentBuildRecipe = db.Status.PotentialBuildRecipes[0]
	//and remove if from the potential list
	db.Status.PotentialBuildRecipes = db.Status.PotentialBuildRecipes[1:]

	tr := pipelinev1beta1.PipelineRun{}
	tr.Namespace = db.Namespace
	tr.GenerateName = db.Name + "-build-"
	tr.Labels = map[string]string{artifactbuildrequest.DependencyBuildIdLabel: db.Labels[artifactbuildrequest.DependencyBuildIdLabel], PipelineRunLabel: ""}
	tr.Spec.PipelineRef = &pipelinev1beta1.PipelineRef{Name: "run-component-build"}
	tr.Spec.Params = []pipelinev1beta1.Param{
		{Name: PipelineScmUrl, Value: pipelinev1beta1.ArrayOrString{Type: pipelinev1beta1.ParamTypeString, StringVal: db.Spec.ScmInfo.SCMURL}},
		{Name: PipelineScmTag, Value: pipelinev1beta1.ArrayOrString{Type: pipelinev1beta1.ParamTypeString, StringVal: db.Spec.ScmInfo.Tag}},
		{Name: PipelinePath, Value: pipelinev1beta1.ArrayOrString{Type: pipelinev1beta1.ParamTypeString, StringVal: db.Spec.ScmInfo.Path}},
		{Name: PipelineImage, Value: pipelinev1beta1.ArrayOrString{Type: pipelinev1beta1.ParamTypeString, StringVal: db.Status.CurrentBuildRecipe.Image}},
	}
	quantity, err := resource.ParseQuantity("1Gi")
	if err != nil {
		return reconcile.Result{}, err
	}
	tr.Spec.Workspaces = []pipelinev1beta1.WorkspaceBinding{
		{Name: "maven-settings", EmptyDir: &v1.EmptyDirVolumeSource{}},
		{Name: "shared-workspace", VolumeClaimTemplate: &v1.PersistentVolumeClaim{Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Resources:   v1.ResourceRequirements{Requests: map[v1.ResourceName]resource.Quantity{v1.ResourceStorage: quantity}}}}},
	}
	if err := controllerutil.SetOwnerReference(db, &tr, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	db.Status.State = v1alpha1.DependencyBuildStateBuilding
	if err = r.client.Status().Update(ctx, db); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, r.client.Create(ctx, &tr)
}

func (r *ReconcileDependencyBuild) handleStateBuilding(ctx context.Context, depId string, db *v1alpha1.DependencyBuild) (reconcile.Result, error) {
	//make sure we still have a linked pr
	list := &pipelinev1beta1.PipelineRunList{}
	lbls := map[string]string{
		artifactbuildrequest.DependencyBuildIdLabel: depId,
	}
	listOpts := &client.ListOptions{
		Namespace:     db.Namespace,
		LabelSelector: labels.SelectorFromSet(lbls),
	}
	if err := r.client.List(ctx, list, listOpts); err != nil {
		return reconcile.Result{}, err
	}
	if len(list.Items) == 0 {
		//no linked pr, back to new
		r.eventRecorder.Eventf(&db, v1.EventTypeWarning, "NoPipelineRun", "The DependencyBuild %s/%s did not have any PipelineRuns", db.Namespace, db.Name)
		db.Status.State = v1alpha1.DependencyBuildStateNew
		return reconcile.Result{}, r.client.Update(ctx, db)
	}
	var pr *pipelinev1beta1.PipelineRun
	//look for the most recent one, there could be multiple builds if earlier recipes failed
	for _, current := range list.Items {
		if pr == nil || pr.CreationTimestamp.Before(&current.CreationTimestamp) {
			pr = &current
		}
	}
	if pr.Name == db.Status.LastCompletedPipelineRun {
		//we have already seen this result
		return reconcile.Result{}, nil
	}

	//if there is no label then ignore it
	if pr.Status.CompletionTime != nil {
		db.Status.LastCompletedPipelineRun = pr.Name
		//the pr is done, lets potentially update the dependency build
		//we just set the state here, the ABR logic is in the ABR controller
		//this keeps as much of the logic in one place as possible

		var contaminates []string
		for _, r := range pr.Status.PipelineResults {
			if r.Name == "contaminants" && len(r.Value) > 0 {
				contaminates = strings.Split(r.Value, ",")
			}
		}
		success := pr.Status.GetCondition(apis.ConditionSucceeded).IsTrue()
		if success {
			if len(contaminates) == 0 {
				db.Status.State = v1alpha1.DependencyBuildStateComplete
			} else {
				//the dependency was contaminated with community deps
				//most likely shaded in
				db.Status.State = v1alpha1.DependencyBuildStateContaminated
				db.Status.Contaminants = contaminates
			}
		} else {
			if len(db.Status.PotentialBuildRecipes) > 0 {
				//we failed this time, attempt a new build recipe
				return r.attemptNewRecipe(ctx, db)
			}
			db.Status.State = v1alpha1.DependencyBuildStateFailed
		}
		err := r.client.Status().Update(ctx, db)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileDependencyBuild) handleStateContaminated(ctx context.Context, db *v1alpha1.DependencyBuild) (reconcile.Result, error) {
	contaminants := db.Status.Contaminants
	if len(contaminants) == 0 {
		//all fixed, just set the state back to new and try again
		//this is triggered when contaminants are removed by the ABR controller
		db.Status.State = v1alpha1.DependencyBuildStateNew
		return reconcile.Result{}, r.client.Update(ctx, db)
	}
	//we want to rebuild the contaminants from source
	//so we create ABRs for them
	//if they already exist we link to the ABR
	for _, contaminant := range contaminants {
		if len(contaminant) == 0 {
			continue
		}
		abrName := artifactbuildrequest.CreateABRName(contaminant)
		abr := v1alpha1.ArtifactBuildRequest{}
		//look for existing ABR
		err := r.client.Get(ctx, types.NamespacedName{Name: abrName, Namespace: db.Namespace}, &abr)
		suffix := hashToString(contaminant)[0:20]
		if err != nil {
			//we just assume this is because it does not exist
			//TODO: how to check the type of the error?
			abr.Spec = v1alpha1.ArtifactBuildRequestSpec{GAV: contaminant}
			abr.Name = abrName
			abr.Namespace = db.Namespace
			abr.Annotations = map[string]string{}
			//use this annotation to link back to the dependency build
			abr.Annotations[artifactbuildrequest.DependencyBuildContaminatedBy+suffix] = db.Name
			err := r.client.Create(ctx, &abr)
			if err != nil {
				return reconcile.Result{}, err
			}
		} else {
			abr.Annotations = map[string]string{}
			abr.Annotations[artifactbuildrequest.DependencyBuildContaminatedBy+suffix] = db.Name
			err := r.client.Update(ctx, &abr)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}
	return reconcile.Result{}, nil
}
