package com.migoku.widget

import android.app.PendingIntent
import android.appwidget.AppWidgetManager
import android.appwidget.AppWidgetProvider
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.graphics.Bitmap
import android.view.View
import android.widget.RemoteViews
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.concurrent.TimeUnit

class MigokuWidgetProvider : AppWidgetProvider() {

    override fun onUpdate(
        context: Context,
        appWidgetManager: AppWidgetManager,
        appWidgetIds: IntArray
    ) {
        for (appWidgetId in appWidgetIds) {
            MigokuWidgetWorker.enqueueImmediateUpdate(context, appWidgetId)
        }
    }

    override fun onReceive(context: Context, intent: Intent) {
        super.onReceive(context, intent)
        if (intent.action == ACTION_REFRESH) {
            val appWidgetManager = AppWidgetManager.getInstance(context)
            val thisWidget = ComponentName(context, MigokuWidgetProvider::class.java)
            val allWidgetIds = appWidgetManager.getAppWidgetIds(thisWidget)
            for (widgetId in allWidgetIds) {
                MigokuWidgetWorker.enqueueImmediateUpdate(context, widgetId)
            }
        }
    }

    override fun onEnabled(context: Context) {
        super.onEnabled(context)
        // Schedule periodic background refresh every 30 minutes
        val periodicRequest = PeriodicWorkRequestBuilder<MigokuWidgetWorker>(
            30, TimeUnit.MINUTES
        ).build()

        WorkManager.getInstance(context).enqueueUniquePeriodicWork(
            WORK_NAME_PERIODIC,
            ExistingPeriodicWorkPolicy.UPDATE,
            periodicRequest
        )
    }

    override fun onDisabled(context: Context) {
        super.onDisabled(context)
        WorkManager.getInstance(context).cancelUniqueWork(WORK_NAME_PERIODIC)
    }

    companion object {
        const val ACTION_REFRESH = "com.migoku.widget.ACTION_REFRESH"
        const val WORK_NAME_PERIODIC = "MigokuPeriodicWidgetUpdate"
        const val PREFS_NAME = "MigokuWidgetPrefs"

        fun updateWidgetViews(
            context: Context,
            appWidgetManager: AppWidgetManager,
            appWidgetId: Int,
            progress: JapaneseProgress?,
            avatar: Bitmap?,
            errorMessage: String? = null
        ) {
            val views = RemoteViews(context.packageName, R.layout.widget_migoku)

            // Setup refresh button click pending intent
            val refreshIntent = Intent(context, MigokuWidgetProvider::class.java).apply {
                action = ACTION_REFRESH
            }
            val refreshPendingIntent = PendingIntent.getBroadcast(
                context,
                appWidgetId,
                refreshIntent,
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
            )
            views.setOnClickPendingIntent(R.id.btn_refresh, refreshPendingIntent)

            // Setup widget body click to open config activity
            val configIntent = Intent(context, WidgetConfigActivity::class.java).apply {
                putExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, appWidgetId)
            }
            val configPendingIntent = PendingIntent.getActivity(
                context,
                appWidgetId,
                configIntent,
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
            )
            views.setOnClickPendingIntent(R.id.widget_container, configPendingIntent)

            // Set Migaku avatar if available
            if (avatar != null) {
                views.setImageViewBitmap(R.id.img_migaku_avatar, avatar)
            } else {
                views.setImageViewResource(R.id.img_migaku_avatar, R.drawable.ic_migaku_logo)
            }

            if (errorMessage != null) {
                views.setViewVisibility(R.id.layout_stats_content, View.GONE)
                views.setViewVisibility(R.id.tv_error_message, View.VISIBLE)
                views.setTextViewText(R.id.tv_error_message, errorMessage)
            } else if (progress != null) {
                views.setViewVisibility(R.id.layout_stats_content, View.VISIBLE)
                views.setViewVisibility(R.id.tv_error_message, View.GONE)

                // Known count
                views.setTextViewText(
                    R.id.tv_known_count,
                    String.format(Locale.getDefault(), "%,d", progress.knownCount)
                )

                // Learning count
                views.setTextViewText(
                    R.id.tv_learning_count,
                    String.format(Locale.getDefault(), "%,d", progress.learningCount)
                )

                // Total and ratio
                views.setTextViewText(
                    R.id.tv_total_count,
                    "Total: " + String.format(Locale.getDefault(), "%,d", progress.totalWords)
                )
                views.setTextViewText(
                    R.id.tv_progress_percent,
                    "${progress.progressPercentage}%"
                )

                // Progress bar
                views.setProgressBar(
                    R.id.progress_horizontal,
                    100,
                    progress.progressPercentage,
                    false
                )

                // Last updated timestamp
                val timeFormat = SimpleDateFormat("HH:mm", Locale.getDefault())
                views.setTextViewText(
                    R.id.tv_last_updated,
                    "Synced: " + timeFormat.format(Date())
                )
            }

            appWidgetManager.updateAppWidget(appWidgetId, views)
        }
    }
}
