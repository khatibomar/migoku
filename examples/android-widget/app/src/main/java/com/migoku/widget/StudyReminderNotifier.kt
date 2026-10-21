package com.migoku.widget

import android.content.Context
import java.time.LocalDate

/**
 * Decides whether the study reminder should fire, using the review forecast.
 *
 * A live fetch is preferred, but a failed one falls back to the counts cached by
 * the widget's background refresh so a flaky connection cannot silence the
 * reminder.
 */
object StudyReminderNotifier {

    fun notifyIfDue(context: Context) {
        val prefs = context.getSharedPreferences(MigokuWidgetProvider.PREFS_NAME, Context.MODE_PRIVATE)
        val serverUrl = prefs.getString("server_url", "http://10.0.2.2:8080") ?: "http://10.0.2.2:8080"
        val apiKey = prefs.getString("api_key", null)
        val lang = prefs.getString("lang", "ja") ?: "ja"

        val live = MigokuApiClient.fetchSchedule(serverUrl, apiKey, lang)
        val (reviews, newCards) = if (live.isSuccess) {
            val schedule = live.getOrThrow()
            MigokuWidgetProvider.saveSchedule(prefs, schedule)
            schedule.reviews to schedule.newCards
        } else {
            val cached = MigokuWidgetProvider.loadSchedule(prefs) ?: return
            if (cached.date != LocalDate.now().toString()) return
            cached.reviews to cached.newCards
        }

        if (reviews <= 0 && newCards <= 0) return
        StudyNotifications.showStudyReminder(context, reviews, newCards)
    }
}
