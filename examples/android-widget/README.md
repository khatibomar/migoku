# Migoku Android Widget

An Android home screen widget that talks to the Migoku API to display your Japanese learning progress. It tracks your **Known** and **Learning** word counts and renders a widget featuring the official Migaku avatar.

Included in this directory:
1. **Interactive Web Simulator (`index.html`)**: Live preview in your browser via `make examples`.
2. **Native Android App & Widget Project (`app/`)**: Full Kotlin Android project using Jetpack `AppWidgetProvider`, `RemoteViews`, and `WorkManager`.
3. **KWGT (Kustom Widget) Instructions**: For users who want a widget on Android without compiling an APK.

---

## Features

- **Live Japanese Progress**: Fetches your word counts via `GET /api/v1/status/counts?lang=ja`.
- **Known & Learning Counters**: Highlights words mastered vs words in progress.
- **Progress Ratio & Bar**: Visual breakdown of your vocabulary progress.
- **Migaku Avatar**: Renders the official Migaku avatar from GitHub:
  `https://avatars.githubusercontent.com/u/52677511?s=200&v=4`
- **Manual Tap-to-Refresh**: Tap the refresh icon on the widget to update stats on demand.
- **Automatic Background Sync**: Schedules background refresh using Android's WorkManager.
- **Configuration Activity**: Enter your Migoku server URL, API key, and target language directly from your phone.

---

## Quick Preview (In Browser)

To preview and test the widget simulator in your web browser:

```bash
# From repository root
make examples
```

Then open:
👉 **[http://localhost:4173/android-widget/](http://localhost:4173/android-widget/)**

You can enter your Migoku server URL and API key, click **Fetch from API**, or test with **Load Sample Data**.

---

## Native Android Project

### Project Structure

```
examples/android-widget/
├── README.md
├── index.html                           # Browser simulator
├── settings.gradle.kts
├── build.gradle.kts
├── gradle.properties
└── app/
    ├── build.gradle.kts
    └── src/main/
        ├── AndroidManifest.xml
        ├── java/com/migoku/widget/
        │   ├── MigokuApiClient.kt       # API client & GitHub avatar loader
        │   ├── MigokuWidgetProvider.kt  # AppWidgetProvider & RemoteViews logic
        │   ├── MigokuWidgetWorker.kt    # Jetpack WorkManager background worker
        │   └── WidgetConfigActivity.kt  # Setup screen on widget placement
        └── res/
            ├── drawable/                # Widget shapes, progress bars, vector icons
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

Avatar image URL:
```
https://avatars.githubusercontent.com/u/52677511?s=200&v=4
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
