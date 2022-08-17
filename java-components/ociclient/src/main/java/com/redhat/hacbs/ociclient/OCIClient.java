package com.redhat.hacbs.ociclient;

import java.io.Closeable;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.List;

import javax.security.sasl.AuthenticationException;

import org.apache.http.HttpHeaders;
import org.apache.http.client.methods.HttpGet;
import org.apache.http.impl.client.CloseableHttpClient;

import com.fasterxml.jackson.databind.ObjectMapper;

/**
 * A basic OCI registry client
 */
public class OCIClient implements Closeable {

    final CloseableHttpClient underlying;
    private final String basicAuthToken;
    private final ObjectMapper objectMapper = new ObjectMapper();

    private final String host;
    private final String repository;
    private final boolean allowInsecure;

    private volatile String pullToken;

    public OCIClient(CloseableHttpClient underlying, String host, String repository, String basicAuthToken,
            boolean allowInsecure) {
        this.underlying = underlying;
        this.host = host;
        this.basicAuthToken = basicAuthToken;
        this.repository = repository;
        this.allowInsecure = allowInsecure;
    }

    public List<String> listTags() {
        return listTagsInternal(false);
    }

    private List<String> listTagsInternal(boolean refreshDone) {
        try {
            String theToken = pullToken;
            if (theToken == null) {
                theToken = doAuth();
            }
            HttpGet get = new HttpGet("http" + (allowInsecure ? "" : "s") + "://" + host + "/v2/" + repository + "/tags/list");
            if (theToken != null) {
                get.addHeader(HttpHeaders.AUTHORIZATION, "Bearer " + theToken);
            }
            try (var result = underlying.execute(get)) {
                if (result.getStatusLine().getStatusCode() == 200) {
                    return objectMapper.reader().createParser(result.getEntity().getContent())
                            .readValueAs(DiscoveryResponse.class)
                            .getTags();
                } else if (result.getStatusLine().getStatusCode() == 401) {
                    //token may have expired
                    if (!refreshDone && theToken != null) {
                        synchronized (this) {
                            pullToken = null;
                        }
                        doAuth();
                        return listTagsInternal(true);
                    }
                    throw new AuthenticationFailedException("Authentication failed "
                            + new String(result.getEntity().getContent().readAllBytes(), StandardCharsets.UTF_8));
                } else {
                    throw new RuntimeException("Invalid response code " + result.getStatusLine().getStatusCode() + " "
                            + new String(result.getEntity().getContent().readAllBytes(), StandardCharsets.UTF_8) + " "
                            + Arrays.toString(result.getAllHeaders()));
                }
            }
        } catch (IOException e) {
            throw new RuntimeException(e);
        }
    }

    private String doAuth() throws IOException {
        if (basicAuthToken == null) {
            return null;
        }
        String readToken = pullToken;
        if (readToken == null) {
            synchronized (this) {
                if (pullToken == null) {
                    HttpGet get = new HttpGet(
                            "http" + (allowInsecure ? "" : "s") + "://" + host + "/v2/auth?service=" + host + "&scope="
                                    + "repository:" + repository + ":pull");
                    get.addHeader(HttpHeaders.AUTHORIZATION, "Basic " + basicAuthToken);
                    try (var result = underlying.execute(get)) {
                        if (result.getStatusLine().getStatusCode() == 200) {
                            return pullToken = objectMapper.reader().createParser(result.getEntity().getContent())
                                    .readValueAs(TokenResponse.class)
                                    .getToken();
                        } else {
                            throw new AuthenticationException("Invalid response code " + result.getStatusLine().getStatusCode()
                                    + " " + new String(result.getEntity().getContent().readAllBytes(), StandardCharsets.UTF_8));
                        }
                    }
                }
                return pullToken;
            }
        }
        return readToken;
    }

    @Override
    public void close() throws IOException {
        underlying.close();
    }

}
