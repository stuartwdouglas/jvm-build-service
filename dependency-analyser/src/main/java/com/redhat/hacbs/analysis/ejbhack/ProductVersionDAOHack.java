package com.redhat.hacbs.analysis.ejbhack;

import javax.enterprise.context.ApplicationScoped;
import javax.transaction.Transactional;

import org.jboss.da.listings.impl.dao.ProductVersionDAOImpl;

@ApplicationScoped
@Transactional
public class ProductVersionDAOHack extends ProductVersionDAOImpl {
}
