// Cocoa launcher for the macOS .app bundle.
//
// thefeed-client is a CLI Go HTTP server with no native UI. Shipping it
// raw as the CFBundleExecutable means there is no NSApplication event
// loop running, so macOS keeps the Dock icon bouncing forever (treats
// the app as "still launching") and never paints the running-dot under
// the icon.
//
// This launcher fixes that by:
//   1. Running a real NSApplication so macOS treats us as a proper app.
//   2. Spawning thefeed-client as a child process from inside the same
//      bundle (Contents/MacOS/thefeed-client).
//   3. Adding a menu-bar status item with Open / Quit affordances, plus
//      handling Dock-icon clicks so the user can re-open the browser
//      tab without hunting for the URL.
//   4. SIGTERMing the child on Cmd+Q / Quit Thefeed so the Go server
//      gets a chance to flush its disk cache before the process dies.
//
// Build: see Makefile target mac-app. swiftc is compiled per-arch and
// lipo'd into a universal binary placed at Contents/MacOS/Thefeed.

import Cocoa
import Darwin  // kill(2), SIGKILL — SIGTERM fallback in applicationWillTerminate

final class AppDelegate: NSObject, NSApplicationDelegate {
    private var child: Process?
    private var statusItem: NSStatusItem?
    // Matches thefeed-client's default --port flag. If we ever switch
    // to a kernel-picked port we'll need to read it back from the
    // child (e.g. via a runtime.port file the Go side writes).
    private let port = 8080

    func applicationDidFinishLaunching(_ notification: Notification) {
        let bundleDir = Bundle.main.bundlePath + "/Contents/MacOS"
        let binary = bundleDir + "/thefeed-client"

        // Stable per-user data dir. Finder launches the app with cwd=/,
        // so thefeed-client's default --data-dir of ./thefeeddata would
        // otherwise land at the filesystem root.
        let dataDir = NSHomeDirectory() + "/Library/Application Support/Thefeed"
        try? FileManager.default.createDirectory(
            atPath: dataDir, withIntermediateDirectories: true
        )

        // Funnel child stdout+stderr into a log file so failures aren't
        // lost — Finder launches discard the parent's standard streams,
        // so without this any crash inside thefeed-client is invisible.
        let logURL = URL(fileURLWithPath: dataDir).appendingPathComponent("launcher.log")
        if !FileManager.default.fileExists(atPath: logURL.path) {
            FileManager.default.createFile(atPath: logURL.path, contents: nil)
        }
        let logHandle = try? FileHandle(forWritingTo: logURL)
        logHandle?.seekToEndOfFile()

        let task = Process()
        task.executableURL = URL(fileURLWithPath: binary)
        task.arguments = ["--data-dir", dataDir, "--port", "\(port)"]
        if let handle = logHandle {
            task.standardOutput = handle
            task.standardError = handle
        }
        // Child exited → quit the launcher too, otherwise the user
        // sees a Dock icon with no server behind it. terminationHandler
        // runs on a background thread, so hop to main before touching
        // NSApp.
        task.terminationHandler = { _ in
            DispatchQueue.main.async {
                NSApp.terminate(nil)
            }
        }

        do {
            try task.run()
            child = task
        } catch {
            NSLog("Thefeed launcher: failed to spawn \(binary): \(error)")
            NSApp.terminate(nil)
            return
        }

        // Menu-bar status item — a visible affordance for Open / Quit
        // since the .app has no main window of its own. Without this
        // the only way to quit cleanly would be Dock right-click,
        // which doesn't deliver Cmd+Q semantics.
        let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        item.button?.title = "Thefeed"
        let menu = NSMenu()
        let openItem = NSMenuItem(title: "Open Thefeed",
                                  action: #selector(openInBrowser),
                                  keyEquivalent: "")
        openItem.target = self
        menu.addItem(openItem)
        menu.addItem(NSMenuItem.separator())
        menu.addItem(NSMenuItem(title: "Quit Thefeed",
                                action: #selector(NSApplication.terminate(_:)),
                                keyEquivalent: "q"))
        item.menu = menu
        statusItem = item
    }

    @objc private func openInBrowser() {
        if let url = URL(string: "http://127.0.0.1:\(port)") {
            NSWorkspace.shared.open(url)
        }
    }

    // Dock-icon click after the browser tab was closed: re-open it.
    func applicationShouldHandleReopen(_ sender: NSApplication,
                                       hasVisibleWindows flag: Bool) -> Bool {
        openInBrowser()
        return true
    }

    func applicationWillTerminate(_ notification: Notification) {
        // SIGTERM the child so Go's signal handler runs its cleanup
        // (server Shutdown, media-cache flush). If the child already
        // exited (terminationHandler path), child is still set but
        // .isRunning is false, so we drop straight through.
        guard let c = child, c.isRunning else { return }
        c.terminate()
        // Poll for graceful exit. macOS gives the app ~5s during
        // shutdown before force-quitting it, so a 2s budget here
        // leaves headroom for NSApplication teardown after we return.
        let deadline = Date().addingTimeInterval(2.0)
        while c.isRunning && Date() < deadline {
            Thread.sleep(forTimeInterval: 0.05)
        }
        if c.isRunning {
            // SIGTERM ignored — fall through to SIGKILL so the .app
            // doesn't leave an orphan thefeed-client lingering after
            // the Dock icon disappears.
            kill(c.processIdentifier, SIGKILL)
        }
    }
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
// .regular = appears in Dock with running dot, gets a top menu bar.
// .accessory would hide the Dock icon entirely (status-bar-only app),
// which contradicts the user expectation of "see that it's running".
app.setActivationPolicy(.regular)
app.run()
