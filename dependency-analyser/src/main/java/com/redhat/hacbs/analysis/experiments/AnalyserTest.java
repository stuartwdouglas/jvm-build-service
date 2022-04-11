package com.redhat.hacbs.analysis.experiments;


import java.util.Collections;

import javax.enterprise.context.control.ActivateRequestContext;
import javax.inject.Inject;

import org.jboss.da.reports.api.ReportsGenerator;
import org.jboss.da.reports.model.api.SCMLocator;
import org.jboss.da.reports.model.request.SCMReportRequest;

import io.quarkus.runtime.Quarkus;
import io.quarkus.runtime.QuarkusApplication;

public class AnalyserTest implements QuarkusApplication {

    @Inject
    ReportsGenerator reportsGenerator;

    public static void main(String ... args) {
        Quarkus.run(AnalyserTest.class, args);
    }

    @Override
    @ActivateRequestContext
    public int run(String... args) throws Exception {
        var report = reportsGenerator.getReportFromSCM(new SCMReportRequest(Collections.emptySet(), Collections.emptySet(), SCMLocator.generic("/Users/stuart/workspace/gizmo","1.0.5.Final","pom.xml")));
       System.out.println(report.toString());
        return 0;
    }
}
