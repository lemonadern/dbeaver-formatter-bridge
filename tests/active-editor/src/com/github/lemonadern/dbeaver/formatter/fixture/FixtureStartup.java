// Copyright 2026 lemonadern
// SPDX-License-Identifier: Apache-2.0
package com.github.lemonadern.dbeaver.formatter.fixture;

import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import java.lang.reflect.Proxy;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.StandardCopyOption;
import java.time.Duration;
import java.util.List;
import java.util.Properties;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.function.Supplier;
import org.eclipse.jface.resource.ImageDescriptor;
import org.eclipse.jface.preference.PreferenceStore;
import org.eclipse.jface.text.source.ISourceViewer;
import org.eclipse.swt.widgets.Display;
import org.eclipse.ui.IEditorInput;
import org.eclipse.ui.IStartup;
import org.eclipse.ui.IWorkbenchPage;
import org.eclipse.ui.IPersistableElement;
import org.eclipse.ui.PlatformUI;
import org.eclipse.ui.editors.text.EditorsUI;
import org.jkiss.dbeaver.ModelPreferences;
import org.jkiss.dbeaver.model.DBPDataSource;
import org.jkiss.dbeaver.model.DBPDataSourceContainer;
import org.jkiss.dbeaver.model.impl.preferences.SimplePreferenceStore;
import org.jkiss.dbeaver.model.impl.sql.BasicSQLDialect;
import org.jkiss.dbeaver.model.sql.SQLSyntaxManager;
import org.jkiss.dbeaver.runtime.DBWorkbench;
import org.jkiss.dbeaver.ui.editors.sql.SQLEditorSourceViewerConfiguration;
import org.jkiss.dbeaver.ui.editors.sql.syntax.SQLFormattingStrategy;

/** Enabled only in disposable integration installations; never shipped in the plugin site. */
public final class FixtureStartup implements IStartup {
    private static final String SQL = "select a,b from example where id=1";
    private static final String PREFIX = "com.github.lemonadern.dbeaver.formatter.fixture.";
    private static final HttpClient HTTP = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(5)).build();
    private static URI endpoint;
    private static String password;
    private String stage = "startup";
    private final StringBuilder results = new StringBuilder();

    @Override public void earlyStartup() {
        String resultPath = System.getProperty("bridge.fixture.result");
        if (resultPath == null) return;
        Thread runner = new Thread(() -> {
            try {
                test();
                results.append("PASS: active-editor integration completed\n");
            } catch (Throwable e) {
                results.append("FAIL: ").append(stage).append("; ").append(e.getClass().getSimpleName())
                    .append(": ").append(String.valueOf(e.getMessage()).replaceAll("[\\r\\n]", " ")).append('\n');
            }
            try {
                Path result = Path.of(resultPath);
                Path temporary = result.resolveSibling(result.getFileName() + ".writing");
                Files.writeString(temporary, results.toString(), StandardCharsets.UTF_8);
                Files.move(temporary, result, StandardCopyOption.ATOMIC_MOVE);
            } catch (Exception ignored) {}
        }, "bridge-active-editor-fixture");
        runner.setDaemon(true);
        runner.start();
    }

    private void test() throws Exception {
        // Only the old published plugin needs preference initialization here.
        // For the new plugin, leave EditorsUI cold so the first default-indent
        // RPC exercises the production UI-thread initialization path.
        stage = "workbench UI readiness";
        if (Boolean.getBoolean("bridge.fixture.legacy")) {
            ui(() -> EditorsUI.getPreferenceStore());
            pass("legacy editor preferences initialized on UI thread");
        } else {
            ui(() -> null);
        }
        Files.writeString(Path.of(System.getProperty("bridge.fixture.result") + ".ui-ready"), "ready\n", StandardCharsets.UTF_8);
        pass("workbench UI ready for initial default-indent RPC");
        var workspace = DBWorkbench.getPlatform().getWorkspace().getAbsolutePath();
        Path registry = workspace.resolve(".metadata/dbeaver-instance.properties");
        for (int i = 0; i < 120; i++) {
            if (Files.exists(registry)) break;
            Thread.sleep(500);
        }
        Properties props = new Properties();
        try (var reader = Files.newBufferedReader(registry)) { props.load(reader); }
        String portKey = props.stringPropertyNames().stream().filter(k -> k.endsWith(".port") || k.equals("port")).findFirst().orElseThrow();
        password = props.getProperty(portKey.substring(0, portKey.length() - 4) + "password");
        endpoint = URI.create("http://127.0.0.1:" + props.getProperty(portKey) + "/handleCommandLine");
        // Wait until the registered HTTP service accepts commands.
        for (int i = 0; ; i++) {
            try { request("{\"version\":1,\"sql\":\"select 1\"}"); break; }
            catch (Exception e) { if (i == 60) throw e; Thread.sleep(500); }
        }
        IWorkbenchPage page = ui(() -> {
            var workbench = PlatformUI.getWorkbench();
            var intro = workbench.getIntroManager().getIntro();
            if (intro != null) workbench.getIntroManager().closeIntro(intro);
            var window = workbench.getWorkbenchWindows()[0];
            var p = window.getActivePage();
            p.closeAllEditors(false);
            return p;
        });
        expectError("no SQL editor", active(), "active_editor_unavailable");
        FixtureEditor a = ui(() -> open(page, "A"));
        expectError("unassigned connection", active(), "active_editor_no_connection");
        ui(() -> { a.fixtureContainer = container("UPPER", false); return null; });
        expectError("disconnected connection", active(), "connection_disconnected");
        ui(() -> { a.fixtureContainer = container("UPPER", true); return null; });
        String expectedA = ui(() -> guiFormat(a));
        String actualA = expectSQL("connection A", active());
        require(expectedA.equals(actualA), "GUI strategy and active A differ");
        require(actualA.contains("SELECT"), "A did not use uppercase connection preference");
        FixtureEditor b = ui(() -> {
            FixtureEditor e = open(page, "B");
            e.fixtureContainer = container("LOWER", true);
            return e;
        });
        String expectedB = ui(() -> guiFormat(b));
        String actualB = expectSQL("connection B", active());
        require(expectedB.equals(actualB), "GUI strategy and active B differ");
        require(actualB.contains("select") && !actualA.equals(actualB), "connection selection did not change result");
        ui(() -> { page.activate(a); return null; });
        require(actualA.equals(expectSQL("switch B to A", active())), "switch did not restore A");
        stage = "multiple Workbench windows";
        var secondWindow = ui(() -> {
            try {
                var window = PlatformUI.getWorkbench().openWorkbenchWindow(page.getPerspective().getId(), null);
                FixtureEditor editor = open(window.getActivePage(), "second-window");
                editor.fixtureContainer = container("LOWER", true);
                window.getShell().forceActive();
                return window;
            } catch (Exception e) { throw new RuntimeException(e); }
        });
        var selectedWindow = ui(() -> PlatformUI.getWorkbench().getActiveWorkbenchWindow());
        if (selectedWindow == null) {
            expectError("ambiguous windows without an active window", active(), "active_window_ambiguous");
        } else {
            String expected = selectedWindow == secondWindow ? actualB : actualA;
            require(expected.equals(expectSQL("active window among multiple windows", active())), "wrong window selected");
        }
        ui(() -> { secondWindow.close(); page.activate(a); return null; });
        require(actualA.equals(expectSQL("single window after closing second", active())), "single-window recovery failed");
        ui(() -> { try { page.openEditor(new Input("plain"), PREFIX + "plain"); } catch (Exception e) { throw new RuntimeException(e); } return null; });
        expectError("non-SQL editor", active(), "active_editor_unavailable");
        expectError("v1 active mode rejection", "{\"version\":1,\"sql\":\"select 1\",\"activeEditor\":true}", "invalid_request");
        expectError("v2 false rejection", "{\"version\":2,\"sql\":\"select 1\",\"activeEditor\":false}", "invalid_request");
        expectError("conflicting selectors", "{\"version\":2,\"sql\":\"select 1\",\"activeEditor\":true,\"connection\":\"x\"}", "invalid_request");
        expectError("wrong selector type", "{\"version\":2,\"sql\":\"select 1\",\"activeEditor\":\"true\"}", "invalid_request");
        ui(() -> { page.activate(a); return null; });
        stage = "UI timeout";
        CompletableFuture<Void> started = new CompletableFuture<>();
        CountDownLatch release = new CountDownLatch(1);
        Display.getDefault().asyncExec(() -> {
            started.complete(null);
            try { release.await(15, TimeUnit.SECONDS); }
            catch (InterruptedException e) { Thread.currentThread().interrupt(); }
        });
        try {
            started.get(5, TimeUnit.SECONDS);
            expectError("UI timeout", active(), "ui_timeout");
        } finally {
            release.countDown();
        }
        ui(() -> null);
        require(actualA.equals(expectSQL("recovery after UI timeout", active())), "timeout recovery failed");

        // Give the global-indent regression its own blocked interval so the
        // active-editor timeout cannot consume its timeout budget.
        stage = "global indent UI timeout";
        CompletableFuture<Void> globalStarted = new CompletableFuture<>();
        CountDownLatch globalRelease = new CountDownLatch(1);
        Display.getDefault().asyncExec(() -> {
            globalStarted.complete(null);
            try { globalRelease.await(15, TimeUnit.SECONDS); }
            catch (InterruptedException e) { Thread.currentThread().interrupt(); }
        });
        try {
            globalStarted.get(5, TimeUnit.SECONDS);
            JsonObject explicitIndent = request("{\"version\":1,\"sql\":\"select 1\",\"indent\":\"  \"}");
            require(explicitIndent.has("sql") && explicitIndent.get("version").getAsInt() == 1,
                "explicit-indent global formatting must not require the UI thread");
            pass("global explicit indent works while UI is blocked");
            // The published 0.1.0 plugin predates the UI-thread default-indent fix.
            if (!Boolean.getBoolean("bridge.fixture.legacy")) {
                expectError("global default-indent UI timeout", "{\"version\":1,\"sql\":\"select 1\"}", "ui_timeout");
            }
        } finally {
            globalRelease.countDown();
        }
        ui(() -> null);
        JsonObject recoveredDefault = request("{\"version\":1,\"sql\":\"select 1\"}");
        require(recoveredDefault.has("sql") && recoveredDefault.get("version").getAsInt() == 1,
            "default-indent global formatting did not recover after the UI timeout");
        pass("global default indent recovers after UI timeout");
        // CLI path validates its v2 envelope and stdin/stdout against the same live instance.
        stage = "native CLI active mode";
        String cli = System.getProperty("bridge.fixture.cli");
        require(cli != null, "bridge.fixture.cli is required");
        if (cli.startsWith("file:")) cli = Path.of(URI.create(cli)).toString();
        Process process = new ProcessBuilder(cli, "--workspace", workspace.toString(), "--active-editor", "--indent", "  ").start();
        process.getOutputStream().write(SQL.getBytes(StandardCharsets.UTF_8));
        process.getOutputStream().close();
        if (!process.waitFor(15, TimeUnit.SECONDS)) { process.destroyForcibly(); throw new AssertionError("CLI timed out"); }
        String stdout = new String(process.getInputStream().readAllBytes(), StandardCharsets.UTF_8);
        String stderr = new String(process.getErrorStream().readAllBytes(), StandardCharsets.UTF_8);
        require(process.exitValue() == 0 && stderr.isEmpty() && actualA.equals(stdout), "CLI did not return active editor output");
        pass("native CLI --active-editor matches GUI strategy");
    }

    private FixtureEditor open(IWorkbenchPage page, String name) {
        try { return (FixtureEditor) page.openEditor(new Input(name), PREFIX + "sql"); }
        catch (Exception e) { throw new RuntimeException(e); }
    }
    private static String active() { return "{\"version\":2,\"activeEditor\":true,\"sql\":\"" + SQL + "\",\"indent\":\"  \"}"; }
    private String expectSQL(String name, String input) throws Exception {
        stage = name;
        JsonObject result = request(input);
        require(result.has("sql") && result.get("version").getAsInt() == 2, "expected v2 SQL response");
        pass(name);
        return result.get("sql").getAsString();
    }
    private void expectError(String name, String input, String error) throws Exception {
        stage = name;
        JsonObject result = request(input);
        require(result.has("error") && error.equals(result.get("error").getAsString()), "expected " + error + " but got " + result);
        require(result.get("version").getAsInt() == JsonParser.parseString(input).getAsJsonObject().get("version").getAsInt(), "error response version differs from request");
        pass(name);
    }
    private static JsonObject request(String input) throws Exception {
        JsonObject outer = new JsonObject();
        var args = new com.google.gson.JsonArray();
        for (String arg : List.of("bridge-format", "--request", input)) args.add(arg);
        outer.add("args", args);
        var response = HTTP.send(HttpRequest.newBuilder(endpoint).timeout(Duration.ofSeconds(10))
            .header("Authorization", "Bearer " + password).header("Content-Type", "application/json")
            .POST(HttpRequest.BodyPublishers.ofString(outer.toString())).build(), HttpResponse.BodyHandlers.ofString());
        require(response.statusCode() == 200, "HTTP response is not 200");
        return JsonParser.parseString(JsonParser.parseString(response.body()).getAsJsonObject().getAsJsonArray("output").get(0).getAsString()).getAsJsonObject();
    }
    private static DBPDataSourceContainer container(String keywordCase, boolean connected) {
        var store = new SimplePreferenceStore(DBWorkbench.getPlatform().getPreferenceStore()) { @Override public void save() {} };
        store.setValue(ModelPreferences.SQL_FORMAT_KEYWORD_CASE, keywordCase);
        DBPDataSourceContainer[] holder = new DBPDataSourceContainer[1];
        DBPDataSource source = (DBPDataSource) Proxy.newProxyInstance(DBPDataSource.class.getClassLoader(), new Class<?>[]{DBPDataSource.class}, (p,m,a) -> switch(m.getName()) {
            case "getSQLDialect" -> BasicSQLDialect.INSTANCE;
            case "getContainer" -> holder[0];
            case "hashCode" -> System.identityHashCode(p);
            case "equals" -> p == a[0];
            default -> null;
        });
        holder[0] = (DBPDataSourceContainer) Proxy.newProxyInstance(DBPDataSourceContainer.class.getClassLoader(), new Class<?>[]{DBPDataSourceContainer.class}, (p,m,a) -> switch(m.getName()) {
            case "getDataSource" -> connected ? source : null;
            case "isConnected" -> connected;
            case "getPreferenceStore" -> store;
            case "getName", "getId" -> "fixture-" + keywordCase;
            case "hashCode" -> System.identityHashCode(p);
            case "equals" -> p == a[0];
            default -> null;
        });
        return holder[0];
    }
    private static String guiFormat(FixtureEditor editor) {
        SQLSyntaxManager syntax = new SQLSyntaxManager();
        syntax.init(editor.getDataSource().getSQLDialect(), editor.getDataSourceContainer().getPreferenceStore());
        var configuration = new SQLEditorSourceViewerConfiguration(editor, new PreferenceStore(), null) {
            @Override public String[] getIndentPrefixes(ISourceViewer viewer, String contentType) { return new String[]{"  "}; }
        };
        return new SQLFormattingStrategy(null, configuration, syntax).format(SQL, true, "", null);
    }
    private static <T> T ui(Supplier<T> action) throws Exception {
        CompletableFuture<T> result = new CompletableFuture<>();
        Display.getDefault().asyncExec(() -> { try { result.complete(action.get()); } catch (Throwable e) { result.completeExceptionally(e); } });
        return result.get(15, TimeUnit.SECONDS);
    }
    private static void require(boolean condition, String message) { if (!condition) throw new AssertionError(message); }
    private void pass(String message) {
        results.append("PASS: ").append(message).append('\n');
        try {
            Files.writeString(Path.of(System.getProperty("bridge.fixture.result") + ".progress"), results.toString(), StandardCharsets.UTF_8);
        } catch (Exception ignored) {}
    }
    private record Input(String name) implements IEditorInput {
        @Override public boolean exists() { return false; }
        @Override public ImageDescriptor getImageDescriptor() { return null; }
        @Override public String getName() { return name; }
        @Override public IPersistableElement getPersistable() { return null; }
        @Override public String getToolTipText() { return name; }
        @Override public <T> T getAdapter(Class<T> type) { return null; }
    }
}
