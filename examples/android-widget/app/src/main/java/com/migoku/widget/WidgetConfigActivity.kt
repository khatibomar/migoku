package com.migoku.widget

import android.app.Activity
import android.appwidget.AppWidgetManager
import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.widget.Button
import android.widget.EditText
import android.widget.Toast

class WidgetConfigActivity : Activity() {

    private var appWidgetId = AppWidgetManager.INVALID_APPWIDGET_ID
    private lateinit var etServerUrl: EditText
    private lateinit var etApiKey: EditText
    private lateinit var etLang: EditText
    private lateinit var btnSave: Button

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // Set result to CANCELED initially so if user backs out, widget is not placed
        setResult(RESULT_CANCELED)

        setContentView(R.layout.activity_widget_config)

        etServerUrl = findViewById(R.id.et_server_url)
        etApiKey = findViewById(R.id.et_api_key)
        etLang = findViewById(R.id.et_lang)
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

        btnSave.setOnClickListener {
            val serverUrl = etServerUrl.text.toString().trim()
            val apiKey = etApiKey.text.toString().trim()
            val lang = etLang.text.toString().trim().ifEmpty { "ja" }

            if (serverUrl.isEmpty()) {
                Toast.makeText(this, R.string.error_server_url_required, Toast.LENGTH_SHORT).show()
                return@setOnClickListener
            }

            // Save preferences
            prefs.edit()
                .putString("server_url", serverUrl)
                .putString("api_key", apiKey.ifEmpty { null })
                .putString("lang", lang)
                .apply()

            // Trigger immediate widget update
            if (appWidgetId != AppWidgetManager.INVALID_APPWIDGET_ID) {
                MigokuWidgetWorker.enqueueImmediateUpdate(this, appWidgetId)
            } else {
                val appWidgetManager = AppWidgetManager.getInstance(this)
                val ids = appWidgetManager.getAppWidgetIds(
                    android.content.ComponentName(this, MigokuWidgetProvider::class.java)
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
}
