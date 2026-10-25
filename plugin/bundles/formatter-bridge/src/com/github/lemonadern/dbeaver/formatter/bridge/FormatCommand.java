// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0

package com.github.lemonadern.dbeaver.formatter.bridge;

import com.google.gson.Gson;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import org.eclipse.swt.widgets.Display;
import org.eclipse.ui.PlatformUI;
import org.eclipse.ui.editors.text.EditorsUI;
import org.eclipse.ui.texteditor.AbstractDecoratedTextEditorPreferenceConstants;
import org.jkiss.dbeaver.model.DBPDataSource;
import org.jkiss.dbeaver.model.DBPDataSourceContainer;
import org.jkiss.dbeaver.model.cli.CLIAbstractSubcommand;
import org.jkiss.dbeaver.model.cli.CLIException;
import org.jkiss.dbeaver.model.cli.CLIProcessResult;
import org.jkiss.dbeaver.model.impl.sql.BasicSQLDialect;
import org.jkiss.dbeaver.model.preferences.DBPPreferenceStore;
import org.jkiss.dbeaver.model.sql.SQLSyntaxManager;
import org.jkiss.dbeaver.model.sql.format.SQLFormatterConfiguration;
import org.jkiss.dbeaver.model.sql.registry.SQLFormatterConfigurationRegistry;
import org.jkiss.dbeaver.runtime.DBWorkbench;
import org.jkiss.dbeaver.ui.editors.sql.SQLEditor;
import picocli.CommandLine;

import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;

/** String-in/string-out adapter; never executes SQL or opens a database connection. */
@CommandLine.Command(name = "bridge-format", description = "Format SQL through DBeaver Formatter Bridge")
public final class FormatCommand extends CLIAbstractSubcommand {
    private static final Gson JSON = new Gson();
    private static final int MAX_SQL_CHARS = 1_048_576;

    @CommandLine.Option(names = "--request", required = true)
    private String request;

    @Override
    public void run() throws CLIException {
        int version = 1;
        try {
            JsonObject input = parseRequest(request);
            version = input.get("version").getAsInt();
            String sql = string(input, "sql", true);
            if (sql.length() > MAX_SQL_CHARS) {
                throw failure("invalid_request");
            }
            String connection = string(input, "connection", false);
            boolean activeEditor = bool(input, "activeEditor");
            if ((activeEditor && version != 2) || (version == 2 && !activeEditor)
                || (activeEditor && connection != null)) {
                throw failure("invalid_request");
            }
            String indent = string(input, "indent", false);
            if (indent != null && (indent.isEmpty() || indent.length() > 32 || !indent.matches("[ \t]+"))) {
                throw failure("invalid_request");
            }

            var configuration = activeEditor ? activeEditorConfiguration(indent) : configuration(connection, indent);
            var formatter = SQLFormatterConfigurationRegistry.getInstance().createFormatter(configuration);
            if (formatter == null) {
                throw failure("formatter_unavailable");
            }
            String formatted = formatter.format(sql, configuration);
            if (formatted == null) {
                throw failure("formatting_failed");
            }
            JsonObject output = new JsonObject();
            output.addProperty("version", version);
            output.addProperty("sql", formatted);
            context().addResult(JSON.toJson(output));
            // This ends the command-line client, not the running RPC server.
            context().setPostAction(CLIProcessResult.PostAction.SHUTDOWN);
        } catch (FormatFailure e) {
            error(version, e.getMessage());
        } catch (Exception e) {
            // Formatter exceptions can include input SQL or paths. Keep them out of RPC diagnostics.
            error(version, "formatting_failed");
        }
    }

    private static SQLFormatterConfiguration configuration(String connection, String indent) throws FormatFailure {
        var platform = DBWorkbench.getPlatform();
        var store = platform.getPreferenceStore();
        DBPDataSource source = null;
        if (connection != null) {
            DBPDataSourceContainer selected = null;
            for (var project : platform.getWorkspace().getProjects()) {
                var candidate = project.getDataSourceRegistry().getDataSource(connection);
                if (candidate != null) {
                    if (selected != null) {
                        throw failure("connection_ambiguous");
                    }
                    selected = candidate;
                }
            }
            if (selected == null) {
                throw failure("connection_not_found");
            }
            source = selected.getDataSource();
            if (source == null || !selected.isConnected()) {
                throw failure("connection_disconnected");
            }
            store = selected.getPreferenceStore();
        }
        return configuration(source, store, indent);
    }

    private static SQLFormatterConfiguration configuration(
        DBPDataSource source, DBPPreferenceStore store, String indent
    ) throws FormatFailure {
        var syntax = new SQLSyntaxManager();
        syntax.init(source == null ? BasicSQLDialect.INSTANCE : source.getSQLDialect(), store);
        var configuration = new SQLFormatterConfiguration(source, syntax);
        configuration.setIndentString(indent == null ? editorIndent() : indent);
        return configuration;
    }

    private static SQLFormatterConfiguration activeEditorConfiguration(String indent) throws FormatFailure {
        if (!PlatformUI.isWorkbenchRunning()) {
            throw failure("ui_unavailable");
        }
        var display = PlatformUI.getWorkbench().getDisplay();
        if (display.isDisposed()) {
            throw failure("ui_unavailable");
        }
        if (Display.getCurrent() == display) {
            return captureActiveEditor(indent);
        }
        var result = new CompletableFuture<SQLFormatterConfiguration>();
        try {
            display.asyncExec(() -> {
                // A timed-out caller must not leave a queued editor lookup behind.
                if (!result.isDone()) {
                    try {
                        result.complete(captureActiveEditor(indent));
                    } catch (Exception e) {
                        result.completeExceptionally(e);
                    }
                }
            });
            return result.get(2, TimeUnit.SECONDS);
        } catch (TimeoutException e) {
            result.cancel(false);
            throw failure("ui_timeout");
        } catch (InterruptedException e) {
            result.cancel(false);
            Thread.currentThread().interrupt();
            throw failure("ui_unavailable");
        } catch (ExecutionException e) {
            if (e.getCause() instanceof FormatFailure failure) {
                throw failure;
            }
            throw failure("ui_unavailable");
        } catch (org.eclipse.swt.SWTException e) {
            throw failure("ui_unavailable");
        }
    }

    private static SQLFormatterConfiguration captureActiveEditor(String indent) throws FormatFailure {
        var workbench = PlatformUI.getWorkbench();
        var window = workbench.getActiveWorkbenchWindow();
        if (window == null) {
            var windows = workbench.getWorkbenchWindows();
            if (windows.length > 1) {
                throw failure("active_window_ambiguous");
            }
            if (windows.length == 0) {
                throw failure("active_editor_unavailable");
            }
            window = windows[0];
        }
        var page = window.getActivePage();
        if (page == null || !(page.getActiveEditor() instanceof SQLEditor editor)) {
            throw failure("active_editor_unavailable");
        }
        var selected = editor.getDataSourceContainer();
        if (selected == null) {
            throw failure("active_editor_no_connection");
        }
        var source = selected.getDataSource();
        if (source == null || !selected.isConnected()) {
            throw failure("connection_disconnected");
        }
        // Build a fresh syntax/configuration snapshot while the selected tab is fixed on the UI thread.
        // The formatter itself runs on the RPC thread and never touches the editor document.
        return configuration(source, selected.getPreferenceStore(), indent);
    }

    private static JsonObject parseRequest(String request) throws FormatFailure {
        if (request == null || request.length() > 8 * MAX_SQL_CHARS) {
            throw failure("invalid_request");
        }
        JsonObject input;
        try {
            input = JsonParser.parseString(request).getAsJsonObject();
        } catch (RuntimeException e) {
            throw failure("invalid_request");
        }
        JsonElement version = input.get("version");
        if (version == null || !version.isJsonPrimitive() || !version.getAsJsonPrimitive().isNumber()
            || !("1".equals(version.getAsString()) || "2".equals(version.getAsString()))) {
            throw failure("unsupported_version");
        }
        return input;
    }

    private static boolean bool(JsonObject input, String name) throws FormatFailure {
        JsonElement value = input.get(name);
        if (value == null) {
            return false;
        }
        if (!value.isJsonPrimitive() || !value.getAsJsonPrimitive().isBoolean()) {
            throw failure("invalid_request");
        }
        return value.getAsBoolean();
    }

    private static String string(JsonObject input, String name, boolean required) throws FormatFailure {
        JsonElement value = input.get(name);
        if (value == null && !required) {
            return null;
        }
        if (value == null || !value.isJsonPrimitive() || !value.getAsJsonPrimitive().isString()) {
            throw failure("invalid_request");
        }
        return value.getAsString();
    }

    private static String editorIndent() throws FormatFailure {
        var preferences = EditorsUI.getPreferenceStore();
        if (!preferences.getBoolean(AbstractDecoratedTextEditorPreferenceConstants.EDITOR_SPACES_FOR_TABS)) {
            return "\t";
        }
        int width = preferences.getInt(AbstractDecoratedTextEditorPreferenceConstants.EDITOR_TAB_WIDTH);
        if (width < 1 || width > 32) {
            throw failure("invalid_request");
        }
        return " ".repeat(width);
    }

    private void error(int version, String code) {
        JsonObject output = new JsonObject();
        output.addProperty("version", version);
        output.addProperty("error", code);
        context().addResult(JSON.toJson(output));
        context().setPostAction(CLIProcessResult.PostAction.ERROR);
    }

    private static FormatFailure failure(String code) {
        return new FormatFailure(code);
    }

    private static final class FormatFailure extends Exception {
        FormatFailure(String code) {
            super(code);
        }
    }
}
