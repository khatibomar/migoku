package com.migoku.widget

import android.app.AlarmManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.SharedPreferences
import java.util.Calendar

/**
 * Schedules the daily "time to study" alarm.
 *
 * The alarm is deliberately inexact (setAndAllowWhileIdle): exact alarms need
 * a user-granted permission on Android 12+, and a study reminder is happy to
 * arrive a few minutes late. It still fires through Doze, and it is re-armed
 * after every fire, so a single alarm keeps rolling forward day after day.
 */
object StudyReminderScheduler {

    const val ACTION_STUDY_REMINDER = "com.migoku.widget.ACTION_STUDY_REMINDER"
    const val PREF_STUDY_TIME = "study_time"
    const val PREF_REMINDER_ENABLED = "reminder_enabled"
    const val DEFAULT_STUDY_TIME = "20:00"

    private const val REQUEST_CODE = 4201

    fun isEnabled(prefs: SharedPreferences): Boolean =
        prefs.getBoolean(PREF_REMINDER_ENABLED, false)

    /** The configured study time as (hour, minute); falls back to the default when malformed. */
    fun studyTime(prefs: SharedPreferences): Pair<Int, Int> {
        val raw = prefs.getString(PREF_STUDY_TIME, DEFAULT_STUDY_TIME)?.trim()
            ?: return defaultTime()
        val parts = raw.split(":")
        val hour = parts.getOrNull(0)?.trim()?.toIntOrNull() ?: return defaultTime()
        val minute = parts.getOrNull(1)?.trim()?.toIntOrNull() ?: return defaultTime()
        if (hour !in 0..23 || minute !in 0..59) return defaultTime()
        return hour to minute
    }

    /** Normalises user input ("7", "7:5", "19:30", "7:30 PM") into a canonical "HH:mm" string. */
    fun normalizeTime(raw: String): String? {
        var text = raw.trim().uppercase()
        var isPm = false
        var isAm = false
        when {
            text.endsWith("PM") -> {
                isPm = true
                text = text.dropLast(2).trim()
            }
            text.endsWith("AM") -> {
                isAm = true
                text = text.dropLast(2).trim()
            }
        }

        val parts = text.split(":")
        val hour = parts.getOrNull(0)?.trim()?.toIntOrNull() ?: return null
        val minute = parts.getOrNull(1)?.trim()?.toIntOrNull() ?: 0
        if (minute !in 0..59) return null

        if (isPm || isAm) {
            if (hour !in 1..12) return null
            val twelveHour = hour % 12
            val twentyFourHour = if (isPm) twelveHour + 12 else twelveHour
            return String.format("%02d:%02d", twentyFourHour, minute)
        }

        if (hour !in 0..23) return null
        return String.format("%02d:%02d", hour, minute)
    }

    fun scheduleNext(context: Context) {
        val prefs = context.getSharedPreferences(MigokuWidgetProvider.PREFS_NAME, Context.MODE_PRIVATE)
        if (!isEnabled(prefs)) {
            cancel(context)
            return
        }

        val (hour, minute) = studyTime(prefs)
        val alarmManager = context.getSystemService(AlarmManager::class.java) ?: return
        alarmManager.setAndAllowWhileIdle(
            AlarmManager.RTC_WAKEUP,
            nextTriggerAt(hour, minute),
            pendingIntent(context)
        )
    }

    fun cancel(context: Context) {
        val alarmManager = context.getSystemService(AlarmManager::class.java) ?: return
        alarmManager.cancel(pendingIntent(context))
    }

    private fun defaultTime(): Pair<Int, Int> = 20 to 0

    private fun pendingIntent(context: Context): PendingIntent {
        val intent = Intent(context, StudyReminderReceiver::class.java).apply {
            action = ACTION_STUDY_REMINDER
        }
        return PendingIntent.getBroadcast(
            context,
            REQUEST_CODE,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
    }

    private fun nextTriggerAt(hour: Int, minute: Int): Long {
        val now = Calendar.getInstance()
        val next = (now.clone() as Calendar).apply {
            set(Calendar.HOUR_OF_DAY, hour)
            set(Calendar.MINUTE, minute)
            set(Calendar.SECOND, 0)
            set(Calendar.MILLISECOND, 0)
        }
        if (!next.after(now)) {
            next.add(Calendar.DAY_OF_YEAR, 1)
        }
        return next.timeInMillis
    }
}
