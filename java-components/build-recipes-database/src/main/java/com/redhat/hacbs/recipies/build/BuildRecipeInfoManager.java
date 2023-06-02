package com.redhat.hacbs.recipies.build;

import java.io.IOException;
import java.nio.file.Path;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.databind.DeserializationFeature;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.dataformat.yaml.YAMLFactory;
import com.redhat.hacbs.recipies.RecipeManager;

public class BuildRecipeInfoManager implements RecipeManager<PrimaryBuildRecipeInfo> {

    private static final ObjectMapper MAPPER = new ObjectMapper(new YAMLFactory())
            .configure(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES, false)
            .setSerializationInclusion(JsonInclude.Include.NON_DEFAULT);

    @Override
    public PrimaryBuildRecipeInfo parse(Path file) throws IOException {
        PrimaryBuildRecipeInfo buildRecipeInfo = MAPPER.readValue(file.toFile(), PrimaryBuildRecipeInfo.class);
        if (buildRecipeInfo == null) {
            return new PrimaryBuildRecipeInfo(); //can happen with empty files
        }
        return buildRecipeInfo;
    }

    @Override
    public void write(PrimaryBuildRecipeInfo data, Path file) throws IOException {
        MAPPER.writeValue(file.toFile(), data);
    }
}
