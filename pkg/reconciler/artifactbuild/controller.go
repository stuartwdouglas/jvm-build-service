package artifactbuild

import (
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/redhat-appstudio/jvm-build-service/pkg/apis/jvmbuildservice/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
)

func SetupNewReconcilerWithManager(mgr ctrl.Manager) error {
	r := newReconciler(mgr)
	return ctrl.NewControllerManagedBy(mgr).For(&v1alpha1.ArtifactBuild{}).
		Watches(&source.Kind{Type: &v1beta1.PipelineRun{}}, handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
			pipelineRun := o.(*v1beta1.PipelineRun)
			communitArtifacts := false
			if pipelineRun.Status.PipelineResults != nil {
				for _, r := range pipelineRun.Status.PipelineResults {
					if r.Name == PipelineResultJavaCommunityDependencies {
						communitArtifacts = true
					}
				}
			}
			if !communitArtifacts {
				return []reconcile.Request{}
			}
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      pipelineRun.Name,
						Namespace: pipelineRun.Namespace,
					},
				},
			}
		})).
		Watches(&source.Kind{Type: &v1alpha1.DependencyBuild{}}, handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
			dependencyBuild := o.(*v1alpha1.DependencyBuild)

			// check if the DependencyBuild is related to ArtifactBuild
			if dependencyBuild.GetLabels() == nil {
				return []reconcile.Request{}
			}
			_, ok := dependencyBuild.GetLabels()[DependencyBuildIdLabel]
			if !ok {
				return []reconcile.Request{}
			}

			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      dependencyBuild.Name,
						Namespace: dependencyBuild.Namespace,
					},
				},
			}
		})).
		Watches(&source.Kind{Type: &v1alpha1.DependencyBuild{}}, &handler.EnqueueRequestForOwner{OwnerType: &v1alpha1.ArtifactBuild{}, IsController: false}).
		Complete(r)
}
