package com.redhat.hacbs.artifactcache.oldsidecar;

import io.smallrye.config.ConfigMapping;

import java.util.Map;

@ConfigMapping(prefix = "gav.relocation")
public interface GavRelocationConfig {
    Map<String, String> pattern();
}
