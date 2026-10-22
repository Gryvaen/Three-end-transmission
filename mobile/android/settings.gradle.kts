pluginManagement {
    resolutionStrategy {
        eachPlugin {
            when (requested.id.id) {
                "com.android.application" -> useModule("com.android.tools.build:gradle:${requested.version}")
                "org.jetbrains.kotlin.android" -> useModule("org.jetbrains.kotlin:kotlin-gradle-plugin:${requested.version}")
            }
        }
    }
    val flutterSdkPath =
        run {
            val properties = java.util.Properties()
            file("local.properties").inputStream().use { properties.load(it) }
            val flutterSdkPath = properties.getProperty("flutter.sdk")
            require(flutterSdkPath != null) { "flutter.sdk not set in local.properties" }
            flutterSdkPath
        }

    // Flutter's included build normally writes into the SDK. Shared/read-only
    // SDKs can contain another user's outputs, so build a local source snapshot.
    // Key it by contents so SDK upgrades and removed files cannot leave stale code.
    val pluginSource = file("$flutterSdkPath/packages/flutter_tools/gradle")
    val generatedDirectories = setOf("build", ".gradle", ".kotlin", "bin")
    val sourceFiles = pluginSource.walkTopDown()
        .onEnter { it == pluginSource || it.parentFile != pluginSource || it.name !in generatedDirectories }
        .filter { it.isFile }
        .sortedBy { it.relativeTo(pluginSource).invariantSeparatorsPath }
        .toList()
    check(sourceFiles.isNotEmpty()) { "Flutter Gradle plugin sources not found: $pluginSource" }
    val digest = java.security.MessageDigest.getInstance("SHA-256")
    digest.update(pluginSource.canonicalPath.toByteArray(Charsets.UTF_8))
    sourceFiles.forEach { source ->
        digest.update(source.relativeTo(pluginSource).invariantSeparatorsPath.toByteArray(Charsets.UTF_8))
        digest.update(0.toByte())
        digest.update(source.readBytes())
    }
    val snapshotId = digest.digest().joinToString("") { "%02x".format(it) }
    val pluginSnapshot = settingsDir.resolve(".gradle/flutter-tools/$snapshotId")
    val ready = pluginSnapshot.resolve(".lanroom-ready")
    if (!ready.isFile) {
        sourceFiles.forEach { source ->
            val destination = pluginSnapshot.resolve(source.relativeTo(pluginSource).path)
            destination.parentFile.mkdirs()
            source.copyTo(destination, overwrite = true)
        }
        ready.writeText(snapshotId)
    }
    // PluginManagementSpec takes a String; passing a File calls Settings'
    // ordinary includeBuild overload, which is too late for plugin resolution.
    includeBuild(pluginSnapshot.absolutePath) { name = "flutter-tools" }

    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

plugins {
    id("dev.flutter.flutter-plugin-loader") version "1.0.0"
    id("com.android.application") version "8.11.1" apply false
    id("org.jetbrains.kotlin.android") version "2.2.20" apply false
}

include(":app")
