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

        val widgetIds = if (targetWidgetId != AppWidgetManager.INVALID_APPWIDGET_ID) {
            intArrayOf(targetWidgetId)
        } else {
            appWidgetManager.getAppWidgetIds(
                ComponentName(context, MigokuWidgetProvider::class.java)
            )
        }

        statsResult.fold(
            onSuccess = { progress ->
                for (id in widgetIds) {
                    MigokuWidgetProvider.updateWidgetViews(
                        context,
                        appWidgetManager,
                        id,
                        progress
                    )
                }
            },
            onFailure = { error ->
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
        )

        Result.success()
    }

    companion object {
        const val EXTRA_WIDGET_ID = "extra_widget_id"

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