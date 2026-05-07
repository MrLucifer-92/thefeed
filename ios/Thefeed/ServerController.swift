import Foundation
import Mobile
import UIKit

/// Owns the embedded gomobile-backed HTTP server.
/// Restarts on foreground and stops on background since iOS doesn't
/// permit a long-lived background server.
final class ServerController: ObservableObject {
    @Published private(set) var port: Int = 0
    @Published private(set) var lastError: String?

    private var instance: MobileServer?
    private var observers: [NSObjectProtocol] = []

    init() {
        let center = NotificationCenter.default
        observers.append(center.addObserver(
            forName: UIApplication.didEnterBackgroundNotification,
            object: nil, queue: .main
        ) { [weak self] _ in self?.stop() })
        observers.append(center.addObserver(
            forName: UIApplication.willEnterForegroundNotification,
            object: nil, queue: .main
        ) { [weak self] _ in self?.start() })
    }

    deinit {
        observers.forEach(NotificationCenter.default.removeObserver)
        instance?.stop()
    }

    func start() {
        guard instance == nil else { return }
        do {
            let dir = try Self.dataDir()
            var err: NSError?
            guard let s = MobileNewServer(dir.path, &err) else {
                lastError = err?.localizedDescription ?? "server start failed"
                return
            }
            instance = s
            port = Int(s.port())
            lastError = nil
        } catch {
            lastError = error.localizedDescription
        }
    }

    func stop() {
        instance?.stop()
        instance = nil
        port = 0
    }

    private static func dataDir() throws -> URL {
        let docs = try FileManager.default.url(
            for: .documentDirectory,
            in: .userDomainMask,
            appropriateFor: nil,
            create: true
        )
        let dir = docs.appendingPathComponent("thefeeddata", isDirectory: true)
        try FileManager.default.createDirectory(
            at: dir, withIntermediateDirectories: true
        )
        return dir
    }
}
