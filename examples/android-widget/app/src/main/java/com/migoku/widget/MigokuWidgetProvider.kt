package com.migoku.widget

import android.app.PendingIntent
import android.appwidget.AppWidgetManager
import android.appwidget.AppWidgetProvider
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.view.View
import android.widget.RemoteViews
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
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
            errorMessage: String? = null
        ) {
            val views = RemoteViews(context.packageName, R.layout.widget_migoku)

            // Tapping a metric card (or its chevron) triggers a refresh.
            val refreshIntent = Intent(context, MigokuWidgetProvider::class.java).apply {
                action = ACTION_REFRESH
            }
            val refreshPendingIntent = PendingIntent.getBroadcast(
                context,
                appWidgetId,
                refreshIntent,
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
            )
            views.setOnClickPendingIntent(R.id.card_learning, refreshPendingIntent)
            views.setOnClickPendingIntent(R.id.card_known, refreshPendingIntent)

            // Tapping the header opens the configuration screen.
            val configIntent = Intent(context, WidgetConfigActivity::class.java).apply {
                putExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, appWidgetId)
            }
            val configPendingIntent = PendingIntent.getActivity(
                context,
                appWidgetId,
                configIntent,
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
            )
            views.setOnClickPendingIntent(R.id.layout_header, configPendingIntent)

            if (errorMessage != null) {
                views.setViewVisibility(R.id.layout_stats_content, View.GONE)
                views.setViewVisibility(R.id.tv_error_message, View.VISIBLE)
                views.setTextViewText(R.id.tv_error_message, errorMessage)
            } else if (progress != null) {
                views.setViewVisibility(R.id.layout_stats_content, View.VISIBLE)
                views.setViewVisibility(R.id.tv_error_message, View.GONE)

                val locale = Locale.getDefault()
                val total = progress.totalWords

                // Learning card: count + share of the deck
                views.setTextViewText(
                    R.id.tv_learning_count,
                    String.format(locale, "%,d", progress.learningCount)
                )
                views.setTextViewText(
                    R.id.tv_learning_total,
                    String.format(locale, "/ %,d", total)
                )
                views.setProgressBar(
                    R.id.progress_learning,
                    100,
                    ratioPercent(progress.learningCount, total),
                    false
                )

                // Known card: count + share of the deck
                views.setTextViewText(
                    R.id.tv_known_count,
                    String.format(locale, "%,d", progress.knownCount)
                )
                views.setTextViewText(
                    R.id.tv_known_total,
                    String.format(locale, "/ %,d", total)
                )
                views.setProgressBar(
                    R.id.progress_known,
                    100,
                    ratioPercent(progress.knownCount, total),
                    false
                )
            }

            appWidgetManager.updateAppWidget(appWidgetId, views)
        }

        private fun ratioPercent(part: Int, total: Int): Int =
            if (total > 0) ((part.toFloat() / total.toFloat()) * 100).toInt() else 0
    }
}
