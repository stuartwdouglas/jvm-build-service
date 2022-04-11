package com.redhat.hacbs.analysis.ejbhack;

import javax.enterprise.inject.Alternative;
import javax.enterprise.inject.Produces;
import javax.persistence.EntityManager;
import javax.persistence.PersistenceContext;
import javax.persistence.PersistenceUnit;

import org.commonjava.maven.galley.auth.PasswordEntry;
import org.commonjava.maven.galley.spi.auth.PasswordManager;
import org.commonjava.maven.galley.transport.htcli.Http;
import org.commonjava.maven.galley.transport.htcli.HttpImpl;

import io.quarkus.arc.Priority;

public class HackProducer {
    //
    //    @Dependent
    //    @Produces
    //    HttpServletRequest request() {
    //        return null;
    //    }
    //
    //    @Dependent
    //    @Produces
    //    HttpServletResponse response() {
    //        return null;
    //    }

    @Alternative
    @Priority(100)
    @Produces
    public Http http() {
        return new HttpImpl(new PasswordManager() {
            @Override
            public String getPassword(PasswordEntry passwordEntry) {
                return null;
            }
        });
    }
}
