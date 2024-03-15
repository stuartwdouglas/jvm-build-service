package com.redhat.hacbs.artifactcache;

import java.math.BigDecimal;
import java.net.URL;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

import org.junit.jupiter.api.Test;

import com.fasterxml.jackson.databind.ObjectMapper;

import io.fabric8.kubernetes.api.model.PodList;

public class FooTest {

    @Test
    public void doStuff() throws Exception {

        ObjectMapper o = new ObjectMapper();
        var podReader = o.readerFor(PodList.class);
        PodList pods = podReader.readValue(new URL(
                "https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/test-platform-results/pr-logs/pull/redhat-appstudio_jvm-build-service/1413/pull-ci-redhat-appstudio-jvm-build-service-main-jvm-build-service-e2e/1768145162757738496/artifacts/jvm-build-service-e2e/gather-extra/artifacts/pods.json"));
        List<Map.Entry<String, BigDecimal>> items = new ArrayList<>();
        for (var i : pods.getItems()) {
            if (!i.getSpec().getNodeSelector().containsKey("node-role.kubernetes.io/master")) {
                continue;
            }
            if (!i.getStatus().getPhase().equals("Running")) {
                continue;
            }
            var total = BigDecimal.ZERO;
            for (var c : i.getSpec().getContainers()) {
                if (c.getResources() != null && c.getResources().getRequests() != null) {
                    var cpu = c.getResources().getRequests().get("cpu");
                    if (cpu != null) {
                        total = total.add(cpu.getNumericalAmount());
                    }
                }
            }
            items.add(Map.entry(i.getMetadata().getNamespace() + "/" + i.getMetadata().getName(), total));
        }
        BigDecimal total = BigDecimal.ZERO;
        items.sort((a, b) -> b.getValue().compareTo(a.getValue()));
        for (var e : items) {
            total = total.add(e.getValue());
            System.out.println(e.getKey() + " " + e.getValue() + " " + total);
        }
    }
}
