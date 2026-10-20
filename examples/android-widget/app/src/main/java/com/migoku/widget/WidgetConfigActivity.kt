package com.migoku.widget

import android.Manifest
import android.app.Activity
import android.appwidget.AppWidgetManager
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.widget.Button
import android.widget.EditText
import android.widget.Switch
import android.widget.Toast

class WidgetConfigActivity : Activity() {

    private var appWidgetId = AppWidgetManager.INVALID_APPWIDGET_ID
    private lateinit var etServerUrl: EditText
    private lateinit var etApiKey: EditText
    private lateinit var etLang: EditText
    private lateinit var etStudyTime: EditText
    private lateinit var switchReminder: Switch
    private lateinit var btnSave: Button

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // Set result to CANCELED initially so if user backs out, widget is not placed
        setResult(RESULT_CANCELED)

        setContentView(R.layout.activity_widget_config)

        etServerUrl = findViewById(R.id.et_server_url)
        etApiKey = findViewById(R.id.et_api_key)
        etLang = findViewById(R.id.et_lang)
        etStudyTime = findViewById(R.id.et_study_time)
        switchReminder = findViewById(R.id.switch_reminder)
        btnSave = findViewById(R.id.btn_save_config)

        // Extract widget ID from launch intent
        val intent = intent
        val extras = intent.extras
        if (extras != null) {
            appWidgetId = extras.getInt(
                AppWidgetManager.EXTRA_APPWIDGET_ID,
                AppWidgetManager.INVALID_APPWIDGET_ID
            )
        }

        // Load existing saved preferences if available
        val prefs = getSharedPreferences(MigokuWidgetProvider.PREFS_NAME, Context.MODE_PRIVATE)
        etServerUrl.setText(prefs.getString("server_url", "http://10.0.2.2:8080"))
        etApiKey.setText(prefs.getString("api_key", ""))
        etLang.setText(prefs.getString("lang", "ja"))
        etStudyTime.setText(prefs.getString(StudyReminderScheduler.PREF_STUDY_TIME, StudyReminderScheduler.DEFAULT_STUDY_TIME))
        switchReminder.isChecked = StudyReminderScheduler.isEnabled(prefs)

        btnSave.setOnClickListener {
            val serverUrl = etServerUrl.text.toString().trim()
            val apiKey = etApiKey.text.toString().trim()
            val lang = etLang.text.toString().trim().ifEmpty { "ja" }
            val rawStudyTime = etStudyTime.text.toString().trim()
            val reminderEnabled = switchReminder.isChecked

            if (serverUrl.isEmpty()) {
                Toast.makeText(this, R.string.error_server_url_required, Toast.LENGTH_SHORT).show()
                return@setOnClickListener
            }

            val studyTime = if (rawStudyTime.isEmpty()) {
                StudyReminderScheduler.DEFAULT_STUDY_TIME
            } else {
                StudyReminderScheduler.normalizeTime(rawStudyTime)
            }
            if (studyTime == null) {
                Toast.makeText(this, R.string.error_study_time_invalid, Toast.LENGTH_SHORT).show()
                return@setOnClickListener
            }

            // Save preferences
            prefs.edit()
                .putString("server_url", serverUrl)
                .putString("api_key", apiKey.ifEmpty { null })
                .putString("lang", lang)
                .putString(StudyReminderScheduler.PREF_STUDY_TIME, studyTime)
                .putBoolean(StudyReminderScheduler.PREF_REMINDER_ENABLED, reminderEnabled)
                .apply()

            // Arm (or disarm) the daily study reminder.
            if (reminderEnabled) {
                ensureNotificationPermission()
            }
            StudyReminderScheduler.scheduleNext(this)

            // Trigger immediate widget update
            if (appWidgetId != AppWidgetManager.INVALID_APPWIDGET_ID) {
                MigokuWidgetWorker.enqueueImmediateUpdate(this, appWidgetId)
            } else {
                val appWidgetManager = AppWidgetManager.getInstance(this)
                val ids = appWidgetManager.getAppWidgetIds(
                    ComponentName(this, MigokuWidgetProvider::class.java)
                )
                for (id in ids) {
                    MigokuWidgetWorker.enqueueImmediateUpdate(this, id)
                }
            }

            // Return success result
            val resultValue = Intent().apply {
                putExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, appWidgetId)
            }
            setResult(RESULT_OK, resultValue)
            Toast.makeText(this, R.string.toast_settings_saved, Toast.LENGTH_SHORT).show()
            finish()
        }
    }

    /**
     * Android 13+ requires an explicit runtime grant before any notification can
     * be posted. The request is fire-and-forget: the reminder still gets armed
     * and will simply be silent until the user grants it in system settings.
     */
    private fun ensureNotificationPermission() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return
        val granted = checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) ==
            PackageManager.PERMISSION_GRANTED
        if (!granted) {
            requestPermissions(
                arrayOf(Manifest.permission.POST_NOTIFICATIONS),
                REQUEST_POST_NOTIFICATIONS
            )
        }
    }

    override fun onRequestPermissionsResult(
        requestCode: Int,
        permissions: Array<out String>,
        grantResults: IntArray
    ) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode != REQUEST_POST_NOTIFICATIONS) return

        val granted = grantResults.isNotEmpty() &&
            grantResults[0] == PackageManager.PERMISSION_GRANTED
        if (!granted) {
            Toast.makeText(this, R.string.toast_notifications_denied, Toast.LENGTH_LONG).show()
        }
    }

    private companion object {
        const val REQUEST_POST_NOTIFICATIONS = 100
    }
}
