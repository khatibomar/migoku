package com.migoku.widget

import android.app.PendingIntent
import android.appwidget.AppWidgetManager
import android.appwidget.AppWidgetProvider
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.SharedPreferences
import android.view.View
import android.widget.RemoteViews
import androidx.work.Constraints
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import java.time.LocalDate
import java.util.Locale
import java.util.concurrent.TimeUnit

class MigokuWidgetProvider : AppWidgetProvider() {

    override fun onUpdate(
        context: Context,
        appWidgetManager: AppWidgetManager,
        appWidgetIds: IntArray
    ) {
        // The periodic job is normally scheduled in onEnabled, but that only runs
        // when the first widget is added. Re-assert it on every update so a lost or
        // cancelled schedule self-heals instead of leaving the widget frozen.
        ensurePeriodicWork(context)

        // Alarms do not survive a reboot, and the app can be updated out from
        // under us; re-arm the study reminder whenever we get a chance.
        StudyReminderScheduler.scheduleNext(context)

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
        ensurePeriodicWork(context)
        StudyReminderScheduler.scheduleNext(context)
    }

    override fun onDisabled(context: Context) {
        super.onDisabled(context)
        WorkManager.getInstance(context).cancelUniqueWork(WORK_NAME_PERIODIC)
    }

    companion object {
        const val ACTION_REFRESH = "com.migoku.widget.ACTION_REFRESH"
        const val WORK_NAME_PERIODIC = "MigokuPeriodicWidgetUpdate"
        const val PREFS_NAME = "MigokuWidgetPrefs"

        private const val PERIODIC_INTERVAL_MINUTES = 30L

        private const val PREF_HAS_LAST_PROGRESS = "has_last_progress"
        private const val PREF_LAST_KNOWN = "last_known"
        private const val PREF_LAST_LEARNING = "last_learning"
        private const val PREF_LAST_UNKNOWN = "last_unknown"
        private const val PREF_LAST_IGNORED = "last_ignored"

        private const val PREF_SCHEDULE_DATE = "schedule_date"
        private const val PREF_SCHEDULE_DUE = "schedule_due_today"
        private const val PREF_SCHEDULE_OVERDUE = "schedule_overdue"

        /**
         * Schedule the background refresh if it is not already scheduled.
         *
         * Uses KEEP so repeated calls (onEnabled, every onUpdate) do not reset an
         * existing run's schedule. The network constraint keeps the job pending
         * while the device is offline instead of running it and failing.
         */
        fun ensurePeriodicWork(context: Context) {
            val constraints = Constraints.Builder()
                .setRequiredNetworkType(NetworkType.CONNECTED)
                .build()

            val periodicRequest = PeriodicWorkRequestBuilder<MigokuWidgetWorker>(
                PERIODIC_INTERVAL_MINUTES, TimeUnit.MINUTES
            )
                .setConstraints(constraints)
                .build()

            WorkManager.getInstance(context).enqueueUniquePeriodicWork(
                WORK_NAME_PERIODIC,
                ExistingPeriodicWorkPolicy.KEEP,
                periodicRequest
            )
        }

        /**
         * Remember the last successful response so a later network blip can keep
         * showing real numbers instead of replacing the widget with an error.
         */
        fun saveLastProgress(prefs: SharedPreferences, progress: JapaneseProgress) {
            prefs.edit()
                .putBoolean(PREF_HAS_LAST_PROGRESS, true)
                .putInt(PREF_LAST_KNOWN, progress.knownCount)
                .putInt(PREF_LAST_LEARNING, progress.learningCount)
                .putInt(PREF_LAST_UNKNOWN, progress.unknownCount)
                .putInt(PREF_LAST_IGNORED, progress.ignoredCount)
                .apply()
        }

        fun loadLastProgress(prefs: SharedPreferences): JapaneseProgress? {
            if (!prefs.getBoolean(PREF_HAS_LAST_PROGRESS, false)) return null

            val known = prefs.getInt(PREF_LAST_KNOWN, 0)
            val learning = prefs.getInt(PREF_LAST_LEARNING, 0)
            val unknown = prefs.getInt(PREF_LAST_UNKNOWN, 0)
            val ignored = prefs.getInt(PREF_LAST_IGNORED, 0)
            val total = known + learning + unknown + ignored
            val percent = if (total > 0) {
                ((known.toFloat() / total.toFloat()) * 100).toInt()
            } else {
                0
            }

            return JapaneseProgress(
                knownCount = known,
                learningCount = learning,
                unknownCount = unknown,
                ignoredCount = ignored,
                totalWords = total,
                progressPercentage = percent
            )
        }

        /**
         * Remember the review forecast so the widget and the study reminder can
         * still show how much is due after the network goes away.
         */
        fun saveSchedule(prefs: SharedPreferences, schedule: StudySchedule) {
            prefs.edit()
                .putInt(PREF_SCHEDULE_DUE, schedule.dueToday)
                .putInt(PREF_SCHEDULE_OVERDUE, schedule.overdue)
                .putString(PREF_SCHEDULE_DATE, LocalDate.now().toString())
                .apply()
        }

        fun loadSchedule(prefs: SharedPreferences): CachedSchedule? {
            val date = prefs.getString(PREF_SCHEDULE_DATE, null) ?: return null
            return CachedSchedule(
                date = date,
                dueToday = prefs.getInt(PREF_SCHEDULE_DUE, 0),
                overdue = prefs.getInt(PREF_SCHEDULE_OVERDUE, 0)
            )
        }

        fun updateWidgetViews(
            context: Context,
            appWidgetManager: AppWidgetManager,
            appWidgetId: Int,
            progress: JapaneseProgress?,
            dueToday: Int = 0,
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

            // How much is due today, straight from the review forecast.
            views.setViewVisibility(R.id.tv_due_today, View.GONE)
            if (errorMessage != null) {
                views.setViewVisibility(R.id.layout_stats_content, View.GONE)
                views.setViewVisibility(R.id.layout_error, View.VISIBLE)
                views.setViewVisibility(R.id.tv_error_message, View.VISIBLE)
                views.setTextViewText(R.id.tv_error_message, errorMessage)
            } else if (progress != null) {
                views.setViewVisibility(R.id.layout_stats_content, View.VISIBLE)
                views.setViewVisibility(R.id.layout_error, View.GONE)
                views.setViewVisibility(R.id.tv_error_message, View.GONE)
                views.setViewVisibility(R.id.tv_due_today, View.VISIBLE)
                views.setTextViewText(
                    R.id.tv_due_today,
                    if (dueToday > 0) {
                        context.resources.getQuantityString(R.plurals.widget_due_today, dueToday, dueToday)
                    } else {
                        context.getString(R.string.widget_due_none)
                    }
                )

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
