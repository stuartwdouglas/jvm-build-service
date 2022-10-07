package com.redhat.hacbs.artifactcache.resources;

import com.redhat.hacbs.artifactcache.services.ArtifactResult;
import com.redhat.hacbs.artifactcache.services.CacheFacade;
import com.redhat.hacbs.resources.util.HashUtil;
import io.quarkus.logging.Log;
import io.smallrye.common.annotation.Blocking;
import org.apache.http.client.utils.DateUtils;
import org.apache.maven.artifact.repository.metadata.Metadata;
import org.apache.maven.artifact.repository.metadata.Versioning;
import org.apache.maven.artifact.repository.metadata.io.xpp3.MetadataXpp3Reader;
import org.apache.maven.artifact.repository.metadata.io.xpp3.MetadataXpp3Writer;
import org.apache.maven.artifact.versioning.ComparableVersion;
import org.apache.maven.model.Model;
import org.apache.maven.model.io.xpp3.MavenXpp3Reader;
import org.apache.maven.model.io.xpp3.MavenXpp3Writer;

import javax.inject.Singleton;
import javax.ws.rs.GET;
import javax.ws.rs.NotFoundException;
import javax.ws.rs.Path;
import javax.ws.rs.PathParam;
import javax.ws.rs.core.HttpHeaders;
import javax.ws.rs.core.Response;
import java.io.BufferedReader;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Date;
import java.util.List;
import java.util.Optional;

@Path("/v1/cache/")
@Blocking
@Singleton
public class CacheMavenResource {

    final CacheFacade cache;

    public CacheMavenResource(CacheFacade cache) {
        this.cache = cache;
    }

    @GET
    @Path("{build-policy}/{commit-time}/{group:.*?}/{artifact}/{version}/{target}")
    public Response get(@PathParam("build-policy") String buildPolicy,
                        @PathParam("group") String group,
                        @PathParam("artifact") String artifact,
                        @PathParam("version") String version, @PathParam("target") String target) throws Exception {
        Log.debugf("Retrieving artifact %s/%s/%s/%s", group, artifact, version, target);
        var result = cache.getArtifactFile(buildPolicy, group, artifact, version, target, true);
        if (result.isPresent()) {
            return createResponse(result);
        }
        Log.infof("Failed to get artifact %s/%s/%s/%s", group, artifact, version, target);
        throw new NotFoundException();
    }

    @GET
    @Path("{build-policy}/{commit-time}/{group:.*?}/maven-metadata.xml{hash:.*?}")
    public InputStream get(@PathParam("build-policy") String buildPolicy,
                           @PathParam("commit-time") long commitTime,
                           @PathParam("group") String group,
                           @PathParam("hash") String hash) throws Exception {
        if (!hash.isEmpty() && !hash.equals(".sha1")) {
            Log.infof("Failed retrieving file %s/%s", group, "maven-metadata.xml" + hash);
            throw new NotFoundException();
        }

        MetadataXpp3Writer writer = new MetadataXpp3Writer();
        ByteArrayOutputStream out = new ByteArrayOutputStream();
        Optional<Metadata> metadata = generateFilteredMetadata(buildPolicy, new Date(commitTime), group);
        if (metadata.isEmpty()) {
            Log.debugf("Failed retrieving file %s/%s", group, "maven-metadata.xml");
            throw new NotFoundException();
        }
        writer.write(out, metadata.get());
        if (!hash.isEmpty()) {
            return new ByteArrayInputStream(HashUtil.sha1(out.toByteArray()).getBytes(StandardCharsets.UTF_8));
        } else {
            return new ByteArrayInputStream(out.toByteArray());
        }
    }


    /**
     * These bintray methods are for builds that reference the now shutdown bintray service. They attempt to find a close
     * version match to a missing artifact so that the build can still proceed.
     */
    @GET
    @Path("{build-policy}/{commit-time}/{quarkusbug:hacbs-bintray}/{group:.*?}/{artifact}/{version}/{target}") //quarkus bug: https://github.com/quarkusio/quarkus/pull/28442
    public Response bintrayGet(@PathParam("build-policy") String buildPolicy,
                               @PathParam("commit-time") long commitTime,
                               @PathParam("group") String group,
                               @PathParam("artifact") String artifact,
                               @PathParam("version") String version, @PathParam("target") String target) throws Exception {
        Log.debugf("Retrieving artifact %s/%s/%s/%s", group, artifact, version, target);
        var result = cache.getArtifactFile(buildPolicy, group, artifact, version, target, true);
        if (result.isPresent()) {
            return createResponse(result);
        }
        ComparableVersion ourVersion = new ComparableVersion(version);
        //not found, look for something close
        //in this case same major version, prefer newer than older
        var optionalMetadata = generateFilteredMetadata(buildPolicy, new Date(0), group + "/" + artifact);
        if (optionalMetadata.isEmpty()) {
            throw new NotFoundException();
        }
        var metadata = optionalMetadata.get();
        if (metadata.getVersioning() == null ||metadata.getVersioning().getVersions() == null) {
            throw new NotFoundException();

        }

        ComparableVersion newer = null;
        ComparableVersion older = null;
        for (var i : metadata.getVersioning().getVersions()) {
            ComparableVersion thisVer = new ComparableVersion(i);
            if (thisVer.compareTo(ourVersion) > 0) {
                if (newer == null || newer.compareTo(thisVer) > 0) { //if newer is larger than this version is closer to the requested
                    newer = thisVer;
                }
            } else {
                if (older == null || older.compareTo(thisVer) < 0) { //if older is smaller than this version is closer to the requested
                    older = thisVer;
                }
            }
        }
        if (older == null && newer == null) {
            throw new NotFoundException();
        }
        String newVersion = newer != null ? newer.toString() : older.toString();
        String dotGroup= group.replaceAll("/", ".");
        Log.infof("Substituting version %s for version %s for artifact %s/%s", newVersion, version, group, artifact);
        target =target.replaceAll(version, newVersion);
        if (target.endsWith(".pom") ) {
            return Response.ok(rewritePom(buildPolicy, group, artifact, newVersion, target, version)).build();
        } else if (target.endsWith(".pom.sha1")) {
            return Response.ok(HashUtil.sha1(rewritePom(buildPolicy, group, artifact, newVersion, target, version))).build();
        } else {
            return get(buildPolicy, group, artifact, newVersion, target);
        }
    }

    private byte[] rewritePom(String buildPolicy, String group, String artifact, String version, String target, String rewriteTarget) throws Exception {
        var result = cache.getArtifactFile(buildPolicy, group, artifact, version, target, true);
        if (result.isEmpty()) {
            throw new NotFoundException();
        }
        try (var ignored = result.get()) {
            MavenXpp3Reader reader = new MavenXpp3Reader();
            Model model = reader.read(new BufferedReader(new InputStreamReader(ignored.getData())));
            model.setVersion(rewriteTarget);
            MavenXpp3Writer writer = new MavenXpp3Writer();
            ByteArrayOutputStream out = new ByteArrayOutputStream();
            writer.write(out, model);
            return out.toByteArray();
        }
    }

    private Response createResponse(Optional<ArtifactResult> result) {
        var builder = Response.ok(result.get().getData());
        if (result.get().getMetadata().containsKey("maven-repo")) {
            builder.header("X-maven-repo", result.get().getMetadata().get("maven-repo"))
                .build();
        }
        if (result.get().getSize() > 0) {
            builder.header(HttpHeaders.CONTENT_LENGTH, result.get().getSize());
        }
        return builder.build();
    }

    @GET
    @Path("{build-policy}/{commit-time}/hacbs-bintray/{group:.*?}/maven-metadata.xml{hash:.*?}")
    public InputStream bintrayGet(@PathParam("build-policy") String buildPolicy,
                                  @PathParam("commit-time") long commitTime,
                                  @PathParam("group") String group,
                                  @PathParam("hash") String hash) throws Exception {
        return get(buildPolicy, commitTime, group, hash);
    }

    private Optional<Metadata> generateFilteredMetadata(String buildPolicy, Date commitTime, String group)
        throws Exception {
        Log.debugf("Retrieving file %s/%s", group, "maven-metadata.xml");
        var data = cache.getMetadataFiles(buildPolicy, group, "maven-metadata.xml");
        if (data.isEmpty()) {
            return Optional.empty();
        }
        if (data.size() == 1) {
            try (var in = data.get(0).getData()) {
                MetadataXpp3Reader reader = new MetadataXpp3Reader();
                return Optional.of(reader.read(in));
            } finally {
                data.get(0).close();
            }
        }

        try {
            //group is not really a group
            //depending on if there are plugins or versions
            //we only care about versions, so we assume the last segment
            //of the group is the artifact id
            int lastIndex = group.lastIndexOf('/');
            String artifactId = group.substring(lastIndex + 1);
            String groupId = group.substring(0, lastIndex);
            Metadata outputModel = null;
            boolean firstFile = true;
            //we need to merge additional versions into a single file
            //we assume the first one is the 'most correct' in terms of the 'release' and 'latest' fields
            for (var i : data) {
                try (var in = i.getData()) {
                    MetadataXpp3Reader reader = new MetadataXpp3Reader();
                    var model = reader.read(in);
                    List<String> versions;
                    if (firstFile) {
                        outputModel = model.clone();
                        if (outputModel.getVersioning() == null) {
                            outputModel.setVersioning(new Versioning());
                        }
                        outputModel.getVersioning().setVersions(new ArrayList<>());
                    }
                    if (model.getVersioning() != null) {
                        String release = null;
                        for (String version : model.getVersioning().getVersions()) {
                            if (commitTime.getTime() > 0) {
                                var result = cache.getArtifactFile(buildPolicy, groupId, artifactId, version,
                                    artifactId + "-" + version + ".pom", false);
                                if (result.isPresent()) {
                                    var lastModified = result.get().getMetadata().get("last-modified");
                                    if (lastModified != null) {
                                        var date = DateUtils.parseDate(lastModified);
                                        if (date != null && date.after(commitTime)) {
                                            //remove versions released after this artifact
                                            Log.infof("Removing version %s from %s/maven-metadata.xml", version, group);
                                        } else {
                                            //TODO: be smarter about how this release version is selected
                                            release = version;
                                            outputModel.getVersioning().getVersions().add(version);
                                        }
                                    } else {
                                        outputModel.getVersioning().getVersions().add(version);
                                    }
                                }
                            } else {
                                outputModel.getVersioning().getVersions().add(version);
                            }
                        }
                        if (firstFile) {
                            outputModel.getVersioning().setRelease(release);
                            outputModel.getVersioning().setLatest(release);
                            outputModel.getVersioning().setLastUpdatedTimestamp(commitTime);
                        }
                    }
                }
                firstFile = false;
            }
            return Optional.of(outputModel);
        } finally {
            for (var i : data) {
                try {
                    i.close();
                } catch (Throwable t) {
                    Log.error("Failed to close resource", t);
                }
            }
        }
    }

}
