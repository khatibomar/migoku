# Migoku Android Widget

An Android home screen widget that talks to the Migoku API to display your Japanese learning progress. It tracks your **Known** and **Learning** word counts on a frosted glass dashboard: hinomaru medallion, `がんばろう！` heading, Mount Fuji / pagoda / cherry blossom scene, and two metric cards.

Included in this directory:
1. **Native Android App & Widget Project (`app/`)**: Full Kotlin Android project using Jetpack `AppWidgetProvider`, `RemoteViews`, and `WorkManager`.
2. **KWGT (Kustom Widget) Instructions**: For users who want a widget on Android without compiling an APK.

---

## Features

- **Live Japanese Progress**: Fetches your word counts via `GET /api/v1/status/counts?lang=ja`.
- **Known & Learning Counters**: Highlights words mastered vs words in progress.
- **Progress Ratio & Bar**: Visual breakdown of your vocabulary progress.
- **Design Plate + Vector Artwork**: the glass background (sky, Mount Fuji, pagoda, cherry blossoms) is the supplied artwork plate in `res/drawable-nodpi/widget_plate.webp`, with rounded corners and a feathered alpha edge so the widget reads as a sheet of glass over the wallpaper; everything on top of it - hinomaru medallion, metric tiles, chevron, progress bars - is vector.
- **Responsive Layout**: columns are driven by weights, so the design holds together from a wide 4x2 placement to a squarer resized one.
- **Manual Tap-to-Refresh**: Tap either metric card to refresh on demand; tap the greeting to reopen the settings screen.
- **Automatic Background Sync**: Schedules background refresh using Android's WorkManager.
- **Configuration Activity**: Enter your Migoku server URL, API key, and target language directly from your phone.

---

## Previewing the Design on a Device

Debug builds ship a preview activity that inflates the widget layout with sample data at
widget size, so you can review the design without placing a widget:

```bash
./build-apk.sh
adb install -r examples/android-widget/app/build/outputs/apk/debug/app-debug.apk
adb shell am start -n com.migoku.widget/.debug.WidgetPreviewActivity --ef scale 3
adb shell screencap -p /sdcard/preview.png
```

Extras: `width_dp` (328), `height_dp` (270), `scale` (1.0), `error` (false). It is a
`src/debug` source set, so it never reaches the release APK.

> The preview uses `LayoutInflater`, not `RemoteViews`. To confirm the layout is
> RemoteViews-safe, watch logcat while a real widget refreshes: `RemoteViews` only
> inflates a fixed allowlist of view classes (no bare `<View>`, no `<Space>`) and
> rejects anything else with *"Class not allowed to be inflated"*.

---

## Native Android Project

### Project Structure

```
examples/android-widget/
├── README.md
├── settings.gradle.kts
├── build.gradle.kts
├── gradle.properties
└── app/
    ├── build.gradle.kts
    └── src/
        ├── debug/                       # Preview activity only, never shipped in release
        │   ├── AndroidManifest.xml
        │   └── java/com/migoku/widget/debug/WidgetPreviewActivity.kt
        └── main/
            ├── AndroidManifest.xml
            ├── java/com/migoku/widget/
            │   ├── MigokuApiClient.kt       # Migoku stats API client
            │   ├── MigokuWidgetProvider.kt  # AppWidgetProvider & RemoteViews logic
            │   ├── MigokuWidgetWorker.kt    # Jetpack WorkManager background worker
            │   └── WidgetConfigActivity.kt  # Setup screen on widget placement
            └── res/
                ├── drawable/                # Vector artwork, card surfaces, progress bars
                ├── drawable-nodpi/          # widget_plate.webp (design background, alpha corners)
                ├── layout/
                │   ├── widget_migoku.xml    # Widget RemoteViews layout
                │   └── activity_widget_config.xml
                ├── values/                  # Strings, colors
                └── xml/
                    └── migoku_widget_info.xml # AppWidget metadata
```

### Running in Android Studio

1. Open **Android Studio**.
2. Select **Open** and choose the `examples/android-widget` directory.
3. Sync Gradle project.
4. Run the app on an Android Emulator or a connected physical device.
5. On the phone's home screen, long-press the wallpaper, tap **Widgets**, find **Migoku Widget**, and drag it onto your home screen.
6. The configuration screen will appear:
   - **Server URL**:
     - Android Emulator: `http://10.0.2.2:8080` (points to `localhost:8080` on host machine)
     - Physical Device: `http://<your-pc-ip>:8080` or your Unikraft Cloud FQDN
   - **API Key**: Your Migoku API key (if `API_SECRET` is configured)
   - **Language**: `ja`
7. Tap **Save & Add Widget**.

### Building APK via Command Line

```bash
cd examples/android-widget
./gradlew assembleDebug
```

The APK will be generated at `app/build/outputs/apk/debug/app-debug.apk`.

### Installing updates (why updates used to be rejected)

Android refuses an in-place update when **either** of these is true:

1. The new APK's `versionCode` is not strictly greater than the installed one.
2. The new APK is not signed with the **same** certificate as the installed one.

Both happened in older builds: `versionCode` was hard-coded to `1`, and CI generated a fresh debug signing key on every run (debug keystores are machine-local and ephemeral), so successive artifacts had different signatures.

This project now avoids that:

- `versionCode` comes from the `ANDROID_VERSION_CODE` environment variable (the GitHub Actions run number in CI) and falls back to the current git commit count for local builds, so every shipped update has a higher version than the last.
- release builds are signed with a **shared keystore kept in GitHub Secrets** (`ANDROID_KEYSTORE_BASE64` and friends), so the certificate never changes between CI builds. The key is not stored in the repository (shipping a private signing key would let anyone forge updates for this applicationId).

> **One-time reset:** APKs produced by the old workflow (or by Android Studio with
> its per-machine debug key) are signed with a different certificate than the
> release builds. Android cannot upgrade across certificates, so you must
> uninstall the old widget once before installing an updated APK. After that,
> future updates install normally.

### Configuring the CI signing key (maintainers)

Generate a keystore once (keep it private, do not commit it):

```bash
cd examples/android-widget
keytool -genkeypair -v -keystore release.keystore -alias migoku \
  -keyalg RSA -keysize 2048 -validity 10000 \
  -storepass <store-pass> -keypass <key-pass> \
  -dname "CN=Migoku Widget, O=Migoku, C=US"
base64 -w 0 release.keystore > release.keystore.b64
```

Then add repository secrets (GitHub → Settings → Secrets and variables → Actions):

- `ANDROID_KEYSTORE_BASE64` — contents of `release.keystore.b64`
- `ANDROID_KEYSTORE_PASSWORD` — `<store-pass>`
- `ANDROID_KEY_ALIAS` — `migoku`
- `ANDROID_KEY_PASSWORD` — `<key-pass>`

---

## API Details

The widget queries the Migoku endpoint:

```http
GET /api/v1/status/counts?lang=ja HTTP/1.1
Host: <migoku-host>
X-Api-Key: <your-api-key>
Accept: application/json
```

Response payload:
```json
[
  {
    "known_count": 1420,
    "learning_count": 85,
    "unknown_count": 280,
    "ignored_count": 30
  }
]
```

---

## Alternative: KWGT (Kustom Widget Maker)

If you already have [KWGT](https://play.google.com/store/apps/details?id=org.kustom.widget) installed on your Android device and prefer a no-code setup:

1. Add a new **KWGT widget** (4x2 or 2x2) to your home screen.
2. In the KWGT editor, add an **Image** element:
   - Set **Bitmap formula**:
     ```
     https://avatars.githubusercontent.com/u/52677511?s=200&v=4
     ```
3. Add a **Text** element for Known words:
   - Set formula:
     ```
     $wg("http://YOUR_MIGOKU_HOST:8080/api/v1/status/counts?lang=ja", json, [0].known_count)$
     ```
4. Add a **Text** element for Learning words:
   - Set formula:
     ```
     $wg("http://YOUR_MIGOKU_HOST:8080/api/v1/status/counts?lang=ja", json, [0].learning_count)$
     ```
5. Set touch action to **Kustom Action → Force RSS/Text/XML update** to enable tap-to-refresh.
