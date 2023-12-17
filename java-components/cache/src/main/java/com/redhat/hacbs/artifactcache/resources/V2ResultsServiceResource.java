package com.redhat.hacbs.artifactcache.resources;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.function.UnaryOperator;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.ws.rs.POST;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.PathParam;
import jakarta.ws.rs.client.ResponseProcessingException;
import jakarta.ws.rs.core.Response;

import org.eclipse.microprofile.config.inject.ConfigProperty;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.databind.ObjectMapper;

import io.fabric8.kubernetes.client.KubernetesClient;
import io.fabric8.kubernetes.client.dsl.Resource;
import io.fabric8.tekton.pipeline.v1.ParamValue;
import io.fabric8.tekton.pipeline.v1.TaskRun;
import io.fabric8.tekton.pipeline.v1.TaskRunResult;
import io.quarkus.logging.Log;
import io.smallrye.common.annotation.Blocking;

@Path("/v2/results-service")
@Blocking
@ApplicationScoped
public class V2ResultsServiceResource {

    final KubernetesClient kubernetesClient;

    final String allowedImage;

    private static final ObjectMapper MAPPER = new ObjectMapper();
    final int retries;

    static {
        MAPPER.setSerializationInclusion(JsonInclude.Include.NON_NULL);
    }

    public V2ResultsServiceResource(KubernetesClient kubernetesClient,
            @ConfigProperty(name = "kube.retries", defaultValue = "5") int retries,
            @ConfigProperty(name = "allowed.results.updater.image", defaultValue = "error-allowed-image-not-defined") String allowedImage) {
        this.kubernetesClient = kubernetesClient;
        this.retries = retries;
        this.allowedImage = allowedImage;
    }

    @POST
    @Path("{name}")
    public Response updateResults(@PathParam("name") String taskRunName, Map<String, String> results) {
        kubeUpdate(taskRunName, results);
        return Response.ok().build();
    }

    private void kubeUpdate(String taskRun, Map<String, String> newResults) {

        for (int i = 0; i <= retries; ++i) {
            try {
                Resource<TaskRun> taskRunResource = kubernetesClient.resources(TaskRun.class)
                        .withName(taskRun);
                boolean found = false;
                //we will only update a task run that contains the build request processor
                for (var step : taskRunResource.get().getSpec().getStepSpecs()) {
                    Object image = step.getAdditionalProperties().get("image");
                    if (image instanceof String) {
                        if (((String) image).contains(allowedImage)) {
                            found = true;
                            break;
                        }
                    }
                }
                if (!found) {
                    throw new RuntimeException("cannot update task that does not contain build-request-processor");
                }
                taskRunResource.editStatus(new UnaryOperator<TaskRun>() {
                    @Override
                    public TaskRun apply(TaskRun taskRun) {
                        List<TaskRunResult> resultsList = new ArrayList<>();
                        if (taskRun.getStatus().getResults() != null) {
                            for (var i : taskRun.getStatus().getResults()) {
                                if (!newResults.containsKey(i.getName())) {
                                    resultsList.add(i);
                                } else {
                                    throw new ResponseProcessingException(Response.status(Response.Status.CONFLICT).build(),
                                            "result " + i.getName() + "already set");
                                }
                            }
                        }
                        for (var e : newResults.entrySet()) {
                            resultsList.add(new TaskRunResult(e.getKey(), "string", new ParamValue(e.getValue())));
                        }
                        taskRun.getStatus().setResults(resultsList);
                        return taskRun;
                    }
                });
                return;
            } catch (Exception e) {
                Log.errorf(e, "Failed to update TaskRun %s, attempt %s of %s", taskRun, i, retries);
            }
        }
    }

}
