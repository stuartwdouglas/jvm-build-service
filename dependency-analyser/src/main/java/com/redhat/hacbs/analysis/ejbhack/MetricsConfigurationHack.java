package com.redhat.hacbs.analysis.ejbhack;

import javax.enterprise.context.ApplicationScoped;
import javax.transaction.Transactional;

import org.jboss.pnc.pncmetrics.MetricsConfiguration;

@ApplicationScoped
@Transactional
public class MetricsConfigurationHack extends MetricsConfiguration {
}
