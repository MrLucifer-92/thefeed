import SwiftUI

/// First-launch language selection. Stored in UserDefaults under "tf.lang".
struct LanguagePickerView: View {
    let onPick: (String) -> Void

    var body: some View {
        ZStack {
            Color(red: 0.07, green: 0.09, blue: 0.13).ignoresSafeArea()

            VStack(spacing: 40) {
                Spacer()

                VStack(spacing: 8) {
                    Text("TheFeed")
                        .font(.system(size: 40, weight: .bold, design: .rounded))
                        .foregroundColor(.white)
                    Text("Choose your language  ·  زبان خود را انتخاب کنید")
                        .font(.callout)
                        .foregroundColor(.white.opacity(0.6))
                        .multilineTextAlignment(.center)
                        .padding(.horizontal, 24)
                }

                VStack(spacing: 16) {
                    LanguageButton(label: "English", subtitle: "Continue in English") {
                        onPick("en")
                    }
                    LanguageButton(label: "فارسی", subtitle: "ادامه به زبان فارسی") {
                        onPick("fa")
                    }
                }
                .padding(.horizontal, 32)

                Spacer()
            }
        }
    }
}

private struct LanguageButton: View {
    let label: String
    let subtitle: String
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            VStack(spacing: 4) {
                Text(label)
                    .font(.system(size: 22, weight: .semibold))
                    .foregroundColor(.white)
                Text(subtitle)
                    .font(.footnote)
                    .foregroundColor(.white.opacity(0.7))
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, 18)
            .background(
                RoundedRectangle(cornerRadius: 16)
                    .fill(Color.white.opacity(0.08))
                    .overlay(
                        RoundedRectangle(cornerRadius: 16)
                            .stroke(Color.white.opacity(0.15), lineWidth: 1)
                    )
            )
        }
    }
}
