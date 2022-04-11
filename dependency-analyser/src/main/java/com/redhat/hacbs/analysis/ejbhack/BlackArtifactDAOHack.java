package com.redhat.hacbs.analysis.ejbhack;

import javax.enterprise.context.ApplicationScoped;
import javax.transaction.Transactional;

import org.jboss.da.listings.impl.dao.BlackArtifactDAOImpl;

@ApplicationScoped
@Transactional
public class BlackArtifactDAOHack extends BlackArtifactDAOImpl {
}
