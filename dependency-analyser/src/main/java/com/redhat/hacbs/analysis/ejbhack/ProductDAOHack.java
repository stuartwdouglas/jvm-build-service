package com.redhat.hacbs.analysis.ejbhack;

import javax.enterprise.context.ApplicationScoped;
import javax.transaction.Transactional;

import org.jboss.da.listings.impl.dao.ProductDAOImpl;

@ApplicationScoped
@Transactional
public class ProductDAOHack extends ProductDAOImpl {
}
