import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "com.migoku.widget"
    compileSdk = 34

    defaultConfig {
        applicationId = "com.migoku.widget"
        minSdk = 26
        targetSdk = 34
        // Android rejects in-place updates unless the new APK has a strictly
        // higher versionCode than the installed one AND is signed with the same
        // key. CI supplies ANDROID_VERSION_CODE (run number) so every build is
        // installable over the previous one; local builds fall back to the git
        // commit count.
        versionCode = migokuVersionCode()
        // Include the git commit (when available) and the Unix timestamp used
        // as versionCode so artifacts built from the same commit stay unique.
        versionName = "1.0.${System.getenv("MIGOKU_GIT_HASH")?.trim() ?: "dev"}-${migokuVersionCode()}"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    // Stable signing is provided through a gitignored keystore.properties (for
    // local builds) or environment variables (used by CI decoding a secret
    // keystore). This keeps the signing key out of the repository: shipping a
    // private key would let anyone forge updates for this applicationId.
    signingConfigs {
        val keystorePropsFile = rootProject.file("keystore.properties")
        val keystoreProps = Properties()
        if (keystorePropsFile.exists()) {
            keystorePropsFile.inputStream().use { keystoreProps.load(it) }
        }

        val storeFileProp = keystoreProps.getProperty("storeFile")?.trim()
        val stableKeystore = if (!storeFileProp.isNullOrBlank()) storeFileProp else System.getenv("MIGOKU_KEYSTORE_PATH")?.trim()

        val storePasswordProp = keystoreProps.getProperty("storePassword")?.trim()
        val stableStorePassword = if (!storePasswordProp.isNullOrBlank()) storePasswordProp else System.getenv("MIGOKU_KEYSTORE_PASSWORD")?.trim()

        val keyAliasProp = keystoreProps.getProperty("keyAlias")?.trim()
        val stableKeyAlias = if (!keyAliasProp.isNullOrBlank()) keyAliasProp else System.getenv("MIGOKU_KEY_ALIAS")?.trim()

        val keyPasswordProp = keystoreProps.getProperty("keyPassword")?.trim()
        val stableKeyPassword = if (!keyPasswordProp.isNullOrBlank()) keyPasswordProp else System.getenv("MIGOKU_KEY_PASSWORD")?.trim()

        val stableSigningReady = !stableKeystore.isNullOrBlank()
            && !stableStorePassword.isNullOrBlank()
            && !stableKeyAlias.isNullOrBlank()
            && !stableKeyPassword.isNullOrBlank()

        if (stableSigningReady) {
            create("stable") {
                storeFile = file(stableKeystore!!)
                storePassword = stableStorePassword
                keyAlias = stableKeyAlias
                keyPassword = stableKeyPassword
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
            // Use the stable key when provided; otherwise fall back to the
            // (per-machine) debug key so release builds still produce an
            // installable, signed APK without stable signing secrets.
            signingConfig = signingConfigs.findByName("stable")
                ?: signingConfigs.getByName("debug")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }
}

dependencies {
    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.work:work-runtime-ktx:2.9.0")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.8.0")
}

fun migokuVersionCode(): Int {
    val fromEnv = System.getenv("ANDROID_VERSION_CODE")?.trim()?.toIntOrNull()
    if (fromEnv != null && fromEnv > 0) return fromEnv
    return try {
        ProcessBuilder("git", "rev-list", "--count", "HEAD")
            .directory(rootDir)
            .start()
            .inputStream
            .bufferedReader()
            .readText()
            .trim()
            .toIntOrNull()
            ?.coerceAtLeast(1) ?: 1
    } catch (e: Exception) {
        1
    }
}
