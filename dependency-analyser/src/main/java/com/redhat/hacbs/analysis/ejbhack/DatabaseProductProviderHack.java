package com.redhat.hacbs.analysis.ejbhack;

import javax.enterprise.context.ApplicationScoped;

import org.jboss.da.products.impl.DatabaseProductProvider;

@DatabaseProductProvider.Database
@ApplicationScoped
public class DatabaseProductProviderHack extends DatabaseProductProvider {
}
