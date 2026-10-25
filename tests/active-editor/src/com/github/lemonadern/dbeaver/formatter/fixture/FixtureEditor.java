// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package com.github.lemonadern.dbeaver.formatter.fixture;

import org.eclipse.core.runtime.IProgressMonitor;
import org.eclipse.swt.widgets.Composite;
import org.eclipse.swt.widgets.Label;
import org.eclipse.ui.IEditorInput;
import org.eclipse.ui.IEditorSite;
import org.jkiss.dbeaver.model.DBPDataSource;
import org.jkiss.dbeaver.model.DBPDataSourceContainer;
import org.jkiss.dbeaver.ui.editors.sql.SQLEditor;

/** Real Workbench tab with a controlled connection, without JDBC/network activity. */
public final class FixtureEditor extends SQLEditor {
    DBPDataSourceContainer fixtureContainer;
    private IEditorInput fixtureInput;
    @Override public void init(IEditorSite site, IEditorInput input) { setSite(site); fixtureInput = input; }
    @Override public IEditorInput getEditorInput() { return fixtureInput; }
    @Override public void createPartControl(Composite parent) { new Label(parent, 0).setText("Bridge integration fixture"); }
    @Override public void setFocus() {}
    @Override public boolean isDirty() { return false; }
    @Override public boolean isSaveAsAllowed() { return false; }
    @Override public void doSave(IProgressMonitor monitor) {}
    @Override public void doSaveAs() {}
    @Override public void dispose() {}
    @Override public DBPDataSourceContainer getDataSourceContainer() { return fixtureContainer; }
    @Override public DBPDataSource getDataSource() { return fixtureContainer == null ? null : fixtureContainer.getDataSource(); }
}
