import SwiftUI

@main
struct ThefeedApp: App {
    @StateObject private var server = ServerController()
    @AppStorage("tf.lang") private var lang: String = ""

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(server)
                .onAppear { server.start() }
                .onDisappear { server.stop() }
                .fullScreenCover(isPresented: Binding(
                    get: { lang.isEmpty },
                    set: { _ in }
                )) {
                    LanguagePickerView { picked in
                        lang = picked
                    }
                }
        }
    }
}
