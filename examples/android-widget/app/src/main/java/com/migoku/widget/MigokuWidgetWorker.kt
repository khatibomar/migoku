package com.migoku.widget

import android.appwidget.AppWidgetManager
import android.content.ComponentName
import android.content.Context
import androidx.work.CoroutineWorker
import androidx.work.Data
import androidx.work.ExistingWorkPolicy
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

class MigokuWidgetWorker(
    private val context: Context,
    workerParams: WorkerParameters
) : CoroutineWorker(context, workerParams) {

    override suspend fun doWork(): Result = withContext(Dispatchers.IO) {
        val appWidgetManager = AppWidgetManager.getInstance(context)
        val targetWidgetId = inputData.getInt(EXTRA_WIDGET_ID, AppWidgetManager.INVALID_APPWIDGET_ID)

        val prefs = context.getSharedPreferences(MigokuWidgetProvider.PREFS_NAME, Context.MODE_PRIVATE)
        val serverUrl = prefs.getString("server_url", "http://10.0.2.2:8080") ?: "http://10.0.2.2:8080"
        val apiKey = prefs.getString("api_key", null)
        val lang = prefs.getString("lang", "ja") ?: "ja"

        // Fetch stats from Migoku API
        val statsResult = MigokuApiClient.fetchJapaneseProgress(serverUrl, apiKey, lang)

        // Fetch the review forecast, which is what the study reminder is built on.
        val scheduleResult = MigokuApiClient.fetchSchedule(serverUrl, apiKey, lang)
        val schedule = scheduleResult.getOrNull()
        if (schedule != null) {
            MigokuWidgetProvider.saveSchedule(prefs, schedule)
            // Keep the daily alarm armed even if the process was killed.
            StudyReminderScheduler.scheduleNext(context)
        }

        val widgetIds = if (targetWidgetId != AppWidgetManager.INVALID_APPWIDGET_ID) {
            intArrayOf(targetWidgetId)
        } else {
            appWidgetManager.getAppWidgetIds(
                ComponentName(context, MigokuWidgetProvider::class.java)
            )
        }

        val error = statsResult.exceptionOrNull()
        if (error == null) {
            val progress = statsResult.getOrThrow()
            // Remember the good response so the next blip can fall back to it.
            MigokuWidgetProvider.saveLastProgress(prefs, progress)
            // The schedule fetch can fail on its own; keep the last known count.
            val dueToday = schedule?.dueToday
                ?: MigokuWidgetProvider.loadSchedule(prefs)?.dueToday
                ?: 0
            for (id in widgetIds) {
                MigokuWidgetProvider.updateWidgetViews(
                    context,
                    appWidgetManager,
                    id,
                    progress,
                    dueToday = dueToday
                )
            }
            return@withContext Result.success()
        }

        // Failure path. Prefer the last good numbers over a dead "can't reach host"
        // screen: a transient network failure should never blank the widget.
        val cached = MigokuWidgetProvider.loadLastProgress(prefs)
        if (cached != null) {
            val cachedSchedule = MigokuWidgetProvider.loadSchedule(prefs)
            for (id in widgetIds) {
                MigokuWidgetProvider.updateWidgetViews(
                    context,
                    appWidgetManager,
                    id,
                    cached,
                    dueToday = cachedSchedule?.dueToday ?: 0
                )
            }
        } else {
            val detail = error.localizedMessage ?: "Error"
            val message = "${context.getString(R.string.status_offline)} ($detail)"
            for (id in widgetIds) {
                MigokuWidgetProvider.updateWidgetViews(
                    context,
                    appWidgetManager,
                    id,
                    progress = null,
                    errorMessage = message
                )
            }
        }

        // Transient failures are retried (with backoff) so the widget recovers on
        // its own; permanent ones (bad key, 404, ...) are not worth hammering.
        val transient = error is MigokuApiException && error.isTransient
        if (transient && runAttemptCount < MAX_TRANSIENT_ATTEMPTS) {
            Result.retry()
        } else {
            Result.success()
        }
    }

    companion object {
        const val EXTRA_WIDGET_ID = "extra_widget_id"
        private const val MAX_TRANSIENT_ATTEMPTS = 3

        fun enqueueImmediateUpdate(context: Context, widgetId: Int) {
            val inputData = Data.Builder()
                .putInt(EXTRA_WIDGET_ID, widgetId)
                .build()

            val request = OneTimeWorkRequestBuilder<MigokuWidgetWorker>()
                .setInputData(inputData)
                .build()

            WorkManager.getInstance(context).enqueueUniqueWork(
                "MigokuImmediateUpdate_$widgetId",
                ExistingWorkPolicy.REPLACE,
                request
            )
        }
    }
}
