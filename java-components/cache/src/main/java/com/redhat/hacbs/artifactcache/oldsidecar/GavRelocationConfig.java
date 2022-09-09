package com.redhat.hacbs.artifactcache.oldsidecar;

import java.util.Map;

import io.smallrye.config.ConfigMapping;

@ConfigMapping(prefix = "gav.relocation")
public interface GavRelocationConfig {
    Map<String, String> pattern();
}
