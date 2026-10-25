// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package com.github.lemonadern.dbeaver.formatter.fixture;

import org.eclipse.core.runtime.IProgressMonitor;
import org.eclipse.swt.widgets.Composite;
import org.eclipse.swt.widgets.Label;
import org.eclipse.ui.IEditorInput;
import org.eclipse.ui.IEditorSite;
import org.eclipse.ui.part.EditorPart;

public final class PlainEditor extends EditorPart {
    @Override public void init(IEditorSite site, IEditorInput input) { setSite(site); setInput(input); }
    @Override public void createPartControl(Composite parent) { new Label(parent, 0).setText("Non-SQL fixture"); }
    @Override public void setFocus() {}
    @Override public boolean isDirty() { return false; }
    @Override public boolean isSaveAsAllowed() { return false; }
    @Override public void doSave(IProgressMonitor monitor) {}
    @Override public void doSaveAs() {}
}
