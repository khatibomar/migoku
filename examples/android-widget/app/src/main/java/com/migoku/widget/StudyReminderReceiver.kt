package com.migoku.widget

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull

/** Fires at the configured study time and posts the "time to study" notification. */
class StudyReminderReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent) {
        val appContext = context.applicationContext

        // Re-arm tomorrow first, so a failure while building the notification
        // can never silence the reminder.
        StudyReminderScheduler.scheduleNext(appContext)

        // goAsync() keeps the process alive for the network round trip; the
        // broadcast itself only grants a few seconds.
        val pendingResult = goAsync()
        CoroutineScope(Dispatchers.IO).launch {
            try {
                withTimeoutOrNull(NOTIFY_TIMEOUT_MS) {
                    StudyReminderNotifier.notifyIfDue(appContext)
                }
            } finally {
                pendingResult.finish()
            }
        }
    }

    private companion object {
        const val NOTIFY_TIMEOUT_MS = 8_000L
    }
}
