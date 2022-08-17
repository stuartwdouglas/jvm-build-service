package com.redhat.hacbs.ociclient;

import java.util.Map;

import org.testcontainers.containers.GenericContainer;

import io.quarkus.logging.Log;
import io.quarkus.test.common.QuarkusTestResourceLifecycleManager;

public class ContainerRegistryTestResourceManager implements QuarkusTestResourceLifecycleManager {

    GenericContainer container;

    @Override
    public Map<String, String> start() {
        int port = startTestRegistry();
        return Map.of("registry", this.container.getHost() + ":" + port);
    }

    private int startTestRegistry() {
        this.container = new GenericContainer("registry:2.7")
                .withReuse(true)
                .withExposedPorts(5000);

        this.container.start();

        Integer port = this.container.getMappedPort(5000);

        Log.debug("\n Test registry details:\n"
                + "\t container name: " + this.container.getContainerName() + "\n"
                + "\t host: " + this.container.getHost() + "\n"
                + "\t port: " + port + "\n"
                + "\t image: " + this.container.getDockerImageName() + "\n");

        return port;
    }

    @Override
    public void stop() {
        this.container.stop();
    }
}
