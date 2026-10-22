package com.github.lemonadern.dbeaver.formatter.bridge;

import com.google.gson.Gson;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import org.eclipse.ui.editors.text.EditorsUI;
import org.eclipse.ui.texteditor.AbstractDecoratedTextEditorPreferenceConstants;
import org.jkiss.dbeaver.model.DBPDataSource;
import org.jkiss.dbeaver.model.DBPDataSourceContainer;
import org.jkiss.dbeaver.model.cli.CLIAbstractSubcommand;
import org.jkiss.dbeaver.model.cli.CLIException;
import org.jkiss.dbeaver.model.cli.CLIProcessResult;
import org.jkiss.dbeaver.model.impl.sql.BasicSQLDialect;
import org.jkiss.dbeaver.model.sql.SQLSyntaxManager;
import org.jkiss.dbeaver.model.sql.format.SQLFormatterConfiguration;
import org.jkiss.dbeaver.model.sql.registry.SQLFormatterConfigurationRegistry;
import org.jkiss.dbeaver.runtime.DBWorkbench;
import picocli.CommandLine;

/** String-in/string-out adapter; never executes SQL or opens a database connection. */
@CommandLine.Command(name = "bridge-format", description = "Format SQL through DBeaver Formatter Bridge")
public final class FormatCommand extends CLIAbstractSubcommand {
    private static final Gson JSON = new Gson();
    private static final int MAX_SQL_CHARS = 1_048_576;

    @CommandLine.Option(names = "--request", required = true)
    private String request;

    @Override
    public void run() throws CLIException {
        try {
            JsonObject input = parseRequest(request);
            String sql = string(input, "sql", true);
            if (sql.length() > MAX_SQL_CHARS) {
                throw failure("invalid_request");
            }
            String connection = string(input, "connection", false);
            String indent = string(input, "indent", false);
            if (indent != null && (indent.isEmpty() || indent.length() > 32 || !indent.matches("[ \t]+"))) {
                throw failure("invalid_request");
            }

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
            var syntax = new SQLSyntaxManager();
            syntax.init(source == null ? BasicSQLDialect.INSTANCE : source.getSQLDialect(), store);
            var configuration = new SQLFormatterConfiguration(source, syntax);
            configuration.setIndentString(indent == null ? editorIndent() : indent);
            var formatter = SQLFormatterConfigurationRegistry.getInstance().createFormatter(configuration);
            if (formatter == null) {
                throw failure("formatter_unavailable");
            }
            String formatted = formatter.format(sql, configuration);
            if (formatted == null) {
                throw failure("formatting_failed");
            }
            JsonObject output = new JsonObject();
            output.addProperty("version", 1);
            output.addProperty("sql", formatted);
            context().addResult(JSON.toJson(output));
            // This ends the command-line client, not the running RPC server.
            context().setPostAction(CLIProcessResult.PostAction.SHUTDOWN);
        } catch (FormatFailure e) {
            error(e.getMessage());
        } catch (Exception e) {
            // Formatter exceptions can include input SQL or paths. Keep them out of RPC diagnostics.
            error("formatting_failed");
        }
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
            || !"1".equals(version.getAsString())) {
            throw failure("unsupported_version");
        }
        return input;
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

    private void error(String code) {
        JsonObject output = new JsonObject();
        output.addProperty("version", 1);
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
