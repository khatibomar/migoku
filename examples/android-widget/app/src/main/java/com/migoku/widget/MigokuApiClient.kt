package com.migoku.widget

import android.util.Log
import org.json.JSONArray
import org.json.JSONObject
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

/** One day of the Migaku review forecast. */
data class StudyScheduleDay(
    val date: String,
    val dueCount: Int,
    val learningCount: Int,
    val knownCount: Int
)

/**
 * The review forecast: how many cards come up for review per day, starting at
 * the SRS active day. `reviews` (studied cards due now) and `newCards` (the
 * never-studied queue the next session would introduce) are what the study
 * reminder is built on.
 */
data class StudySchedule(
    val language: String,
    val today: String,
    val dueToday: Int,
    val learningToday: Int,
    val knownToday: Int,
    val overdue: Int,
    val reviews: Int,
    val newCards: Int,
    val days: List<StudyScheduleDay>
)

/** A previously fetched schedule, kept on the device so a failed refresh can still be served. */
data class CachedSchedule(
    val date: String,
    val dueToday: Int,
    val overdue: Int,
    val reviews: Int,
    val newCards: Int
)

/**
 * Error carrying the HTTP status (null for transport failures) so callers can
 * tell a transient network blip from a permanent problem such as a bad API key.
 */
class MigokuApiException(
    message: String,
    val statusCode: Int? = null
) : Exception(message) {

    /** True when retrying is likely to succeed (network drop, timeout, 5xx). */
    val isTransient: Boolean
        get() = statusCode == null || statusCode == 408 || statusCode == 429 || statusCode >= 500
}

object MigokuApiClient {
    private const val TAG = "MigokuApiClient"

    /**
     * Fetches Japanese language learning counts from Migoku API.
     * Hits GET /api/v1/status/counts?lang=ja
     */
    fun fetchJapaneseProgress(serverUrl: String, apiKey: String?, lang: String = "ja"): Result<JapaneseProgress> {
        return try {
            val cleanUrl = serverUrl.trim().trimEnd('/')
            val responseBody = get("$cleanUrl/api/v1/status/counts?lang=$lang", apiKey)

            val statsObject = parseCountsResponse(responseBody)
                ?: return Result.failure(MigokuApiException("Empty or invalid JSON response from Migoku"))

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
            Result.failure(e.asApiException())
        }
    }

    /**
     * Fetches the review forecast from Migoku API.
     * Hits GET /api/v1/stats/schedule?lang=ja&days=days
     */
    fun fetchSchedule(
        serverUrl: String,
        apiKey: String?,
        lang: String = "ja",
        days: Int = 7
    ): Result<StudySchedule> {
        return try {
            val cleanUrl = serverUrl.trim().trimEnd('/')
            val responseBody = get("$cleanUrl/api/v1/stats/schedule?lang=$lang&days=$days", apiKey)

            val root = JSONObject(responseBody)

            val dayList = mutableListOf<StudyScheduleDay>()
            val daysArray = root.optJSONArray("days")
            if (daysArray != null) {
                for (i in 0 until daysArray.length()) {
                    val day = daysArray.optJSONObject(i) ?: continue
                    dayList.add(
                        StudyScheduleDay(
                            date = day.optString("date"),
                            dueCount = day.optInt("due_count", 0),
                            learningCount = day.optInt("learning_count", 0),
                            knownCount = day.optInt("known_count", 0)
                        )
                    )
                }
            }

            Result.success(
                StudySchedule(
                    language = root.optString("language", lang),
                    today = root.optString("today"),
                    dueToday = root.optInt("due_today", 0),
                    learningToday = root.optInt("learning_today", 0),
                    knownToday = root.optInt("known_today", 0),
                    overdue = root.optInt("overdue", 0),
                    reviews = root.optInt("reviews", 0),
                    newCards = root.optInt("new_cards", 0),
                    days = dayList
                )
            )
        } catch (e: Exception) {
            Log.e(TAG, "Error fetching schedule: ${e.message}", e)
            Result.failure(e.asApiException())
        }
    }

    private fun get(url: String, apiKey: String?): String {
        val conn = (URL(url).openConnection() as HttpURLConnection).apply {
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
            throw MigokuApiException("HTTP $statusCode: $errorStream", statusCode)
        }

        val body = conn.inputStream.bufferedReader().use { it.readText() }
        conn.disconnect()
        return body
    }

    private fun Exception.asApiException(): MigokuApiException =
        if (this is MigokuApiException) {
            this
        } else {
            MigokuApiException(localizedMessage ?: "Network error")
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
}
