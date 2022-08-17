package com.redhat.hacbs.ociclient;

import java.util.List;

public class DiscoveryResponse {

    private String name;
    private List<String> tags;

    public String getName() {
        return name;
    }

    public DiscoveryResponse setName(String name) {
        this.name = name;
        return this;
    }

    public List<String> getTags() {
        return tags;
    }

    public DiscoveryResponse setTags(List<String> tags) {
        this.tags = tags;
        return this;
    }
}
