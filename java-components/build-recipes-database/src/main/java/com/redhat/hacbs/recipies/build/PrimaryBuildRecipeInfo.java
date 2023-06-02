package com.redhat.hacbs.recipies.build;

import java.util.HashMap;
import java.util.Map;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

@JsonIgnoreProperties(ignoreUnknown = true)
public class PrimaryBuildRecipeInfo extends BuildRecipeInfo {

    private Map<String, BuildRecipeInfo> additionalBuilds = new HashMap<>();

    public Map<String, BuildRecipeInfo> getAdditionalBuilds() {
        return additionalBuilds;
    }

    public PrimaryBuildRecipeInfo setAdditionalBuilds(Map<String, BuildRecipeInfo> additionalBuilds) {
        this.additionalBuilds = additionalBuilds;
        return this;
    }

}
