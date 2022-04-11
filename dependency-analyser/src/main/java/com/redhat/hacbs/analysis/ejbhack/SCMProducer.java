package com.redhat.hacbs.analysis.ejbhack;

import javax.enterprise.inject.Produces;

import org.jboss.da.scm.api.SCM;
import org.jboss.da.scm.impl.SCMImpl;

public class SCMProducer {

    @Produces
    SCM scm() {
        return new SCMImpl();
    }
}
