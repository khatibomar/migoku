package com.migoku.widget.debug

import android.app.Activity
import android.graphics.Color
import android.os.Bundle
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.widget.FrameLayout
import android.widget.ProgressBar
import android.widget.TextView
import com.migoku.widget.R

/**
 * Debug-build-only harness that inflates the widget layout at widget size with
 * sample data, so the design can be screenshotted without placing a widget:
 *
 *     ./build-apk.sh
 *     adb install -r examples/android-widget/app/build/outputs/apk/debug/app-debug.apk
 *     adb shell am start -n com.migoku.widget/.debug.WidgetPreviewActivity --ef scale 3
 *     adb shell screencap -p /sdcard/preview.png
 *
 * Extras: `width_dp` (328), `height_dp` (270), `scale` (1.0), `error` (false).
 *
 * Note this uses LayoutInflater, not RemoteViews, so it does not prove the layout
 * is RemoteViews-safe - for that, watch logcat for "Class not allowed to be
 * inflated" while a real widget refreshes on the launcher.
 */
class WidgetPreviewActivity : Activity() {

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val widthDp = intent.getIntExtra(EXTRA_WIDTH_DP, 328)
        val heightDp = intent.getIntExtra(EXTRA_HEIGHT_DP, 270)
        val scale = intent.getFloatExtra(EXTRA_SCALE, 1f).coerceIn(0.25f, 4f)
        val showError = intent.getBooleanExtra(EXTRA_ERROR, false)
        val density = resources.displayMetrics.density

        val host = FrameLayout(this)
        host.setBackgroundColor(Color.parseColor("#1B2430"))
        host.layoutParams = ViewGroup.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT,
            ViewGroup.LayoutParams.MATCH_PARENT
        )

        val widget = layoutInflater.inflate(R.layout.widget_migoku, host, false)
        sample(widget, showError)
        host.addView(
            widget,
            FrameLayout.LayoutParams(
                (widthDp * density * scale).toInt(),
                (heightDp * density * scale).toInt(),
                Gravity.CENTER
            )
        )
        setContentView(host)
    }

    private fun sample(widget: View, showError: Boolean) {
        fun text(id: Int, value: String) = widget.findViewById<TextView>(id).apply { text = value }

        if (showError) {
            text(R.id.tv_error_message, getString(R.string.status_offline))
            widget.findViewById<View>(R.id.tv_error_message).visibility = View.VISIBLE
            widget.findViewById<View>(R.id.layout_error).visibility = View.VISIBLE
            widget.findViewById<View>(R.id.layout_stats_content).visibility = View.GONE
            return
        }

        text(R.id.tv_learning_count, "24")
        text(R.id.tv_learning_total, "/ 1,500")
        text(R.id.tv_known_count, "1,420")
        text(R.id.tv_known_total, "/ 1,500")
        widget.findViewById<ProgressBar>(R.id.progress_learning).progress = 2
        widget.findViewById<ProgressBar>(R.id.progress_known).progress = 95
    }

    companion object {
        const val EXTRA_WIDTH_DP = "width_dp"
        const val EXTRA_HEIGHT_DP = "height_dp"
        const val EXTRA_SCALE = "scale"
        const val EXTRA_ERROR = "error"
    }
}