package com.migoku.widget

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.util.Log
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.io.FileOutputStream
import java.io.InputStream
import java.net.HttpURLConnection
import java.net.URL

data class JapaneseProgress(
    val knownCount: Int,
    val learningCount: Int,
    val unknownCount: Int,
    val ignoredCount: Int,
    val totalWords: Int,
    val progressPercentage: Int
)

object MigokuApiClient {
    private const val TAG = "MigokuApiClient"
    const val DEFAULT_AVATAR_URL = "https://avatars.githubusercontent.com/u/52677511?s=200&v=4"
    private const val AVATAR_CACHE_FILE = "migaku_avatar_cache.png"

    /**
     * Fetches Japanese language learning counts from Migoku API.
     * Hits GET /api/v1/status/counts?lang=ja
     */
    fun fetchJapaneseProgress(serverUrl: String, apiKey: String?, lang: String = "ja"): Result<JapaneseProgress> {
        return try {
            val cleanUrl = serverUrl.trim().trimEnd('/')
            val endpoint = "$cleanUrl/api/v1/status/counts?lang=$lang"
            val url = URL(endpoint)
            val conn = (url.openConnection() as HttpURLConnection).apply {
                requestMethod = "GET"
                connectTimeout = 8000
                readTimeout = 8000
                setRequestProperty("Accept", "application/json")
                if (!apiKey.isNullOrBlank()) {
                    setRequestProperty("X-Api-Key", apiKey.trim())
                }
            }

            val statusCode = conn.responseCode
            if (statusCode !in 200..299) {
                val errorStream = conn.errorStream?.bufferedReader()?.readText() ?: ""
                conn.disconnect()
                return Result.failure(Exception("HTTP $statusCode: $errorStream"))
            }

            val responseBody = conn.inputStream.bufferedReader().use { it.readText() }
            conn.disconnect()

            val statsObject = parseCountsResponse(responseBody)
                ?: return Result.failure(Exception("Empty or invalid JSON response from Migoku"))

            val known = statsObject.optInt("known_count", 0)
            val learning = statsObject.optInt("learning_count", 0)
            val unknown = statsObject.optInt("unknown_count", 0)
            val ignored = statsObject.optInt("ignored_count", 0)
            val total = known + learning + unknown + ignored
            val percent = if (total > 0) ((known.toFloat() / total.toFloat()) * 100).toInt() else 0

            Result.success(
                JapaneseProgress(
                    knownCount = known,
                    learningCount = learning,
                    unknownCount = unknown,
                    ignoredCount = ignored,
                    totalWords = total,
                    progressPercentage = percent
                )
            )
        } catch (e: Exception) {
            Log.e(TAG, "Error fetching progress: ${e.message}", e)
            Result.failure(e)
        }
    }

    private fun parseCountsResponse(json: String): JSONObject? {
        val trimmed = json.trim()
        return when {
            trimmed.startsWith("[") -> {
                val array = JSONArray(trimmed)
                if (array.length() > 0) array.getJSONObject(0) else null
            }
            trimmed.startsWith("{") -> JSONObject(trimmed)
            else -> null
        }
    }

    /**
     * Downloads and caches the Migaku avatar bitmap from GitHub.
     */
    fun getOrFetchAvatar(context: Context, avatarUrl: String = DEFAULT_AVATAR_URL): Bitmap? {
        val cacheFile = File(context.cacheDir, AVATAR_CACHE_FILE)

        // Return cached bitmap if fresh (less than 24 hours old)
        if (cacheFile.exists() && System.currentTimeMillis() - cacheFile.lastModified() < 86_400_000) {
            BitmapFactory.decodeFile(cacheFile.absolutePath)?.let { return it }
        }

        return try {
            val url = URL(avatarUrl)
            val conn = (url.openConnection() as HttpURLConnection).apply {
                connectTimeout = 6000
                readTimeout = 6000
                doInput = true
                connect()
            }

            val input: InputStream = conn.inputStream
            val bitmap = BitmapFactory.decodeStream(input)
            conn.disconnect()

            if (bitmap != null) {
                // Save to cache
                FileOutputStream(cacheFile).use { out ->
                    bitmap.compress(Bitmap.CompressFormat.PNG, 95, out)
                }
            }
            bitmap
        } catch (e: Exception) {
            Log.w(TAG, "Could not fetch avatar: ${e.message}")
            if (cacheFile.exists()) {
                BitmapFactory.decodeFile(cacheFile.absolutePath)
            } else null
        }
    }
}
