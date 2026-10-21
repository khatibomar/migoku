package com.migoku.widget

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build

/** Builds and posts the "time to study" notification. */
object StudyNotifications {

    private const val CHANNEL_ID = "migoku_study_reminder"
    private const val NOTIFICATION_ID = 1001

    fun ensureChannel(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = context.getSystemService(NotificationManager::class.java) ?: return
        if (manager.getNotificationChannel(CHANNEL_ID) != null) return

        val channel = NotificationChannel(
            CHANNEL_ID,
            context.getString(R.string.notification_channel_name),
            NotificationManager.IMPORTANCE_DEFAULT
        ).apply {
            description = context.getString(R.string.notification_channel_description)
        }
        manager.createNotificationChannel(channel)
    }

    fun showStudyReminder(context: Context, reviews: Int, newCards: Int) {
        ensureChannel(context)

        val manager = context.getSystemService(NotificationManager::class.java) ?: return
        if (!manager.areNotificationsEnabled()) return

        val body = buildString {
            if (reviews > 0) {
                append(context.resources.getQuantityString(R.plurals.notification_reviews, reviews, reviews))
            }
            if (newCards > 0) {
                if (isNotEmpty()) append(' ')
                append(context.resources.getQuantityString(R.plurals.notification_new_cards, newCards, newCards))
            }
        }
        if (body.isEmpty()) return

        // Tapping the reminder opens the widget's settings so the study time can
        // be changed without hunting for the widget on the home screen.
        val contentIntent = PendingIntent.getActivity(
            context,
            NOTIFICATION_ID,
            Intent(context, WidgetConfigActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val notification = Notification.Builder(context, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(context.getString(R.string.notification_title))
            .setContentText(body)
            .setStyle(Notification.BigTextStyle().bigText(body))
            .setAutoCancel(true)
            .setContentIntent(contentIntent)
            .build()

        manager.notify(NOTIFICATION_ID, notification)
    }
}
