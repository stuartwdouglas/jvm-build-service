package com.redhat.hacbs.ociclient;

public class TokenResponse {
    public String token;

    public String getToken() {
        return token;
    }

    public TokenResponse setToken(String token) {
        this.token = token;
        return this;
    }
}
