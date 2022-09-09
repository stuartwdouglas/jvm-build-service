package com.redhat.hacbs.artifactcache.services;

import java.util.Collections;
import java.util.List;

public class BuildPolicy {

    final List<Repository> repositories;
    final boolean transformed;

    public BuildPolicy(List<Repository> repositories, boolean transformed) {
        this.repositories = Collections.unmodifiableList(repositories);
        this.transformed = transformed;
    }

    public List<Repository> getRepositories() {
        return repositories;
    }

    public boolean isTransformed() {
        return transformed;
    }
}
