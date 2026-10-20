package com.migoku.widget

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * The study time is typed by hand into the config screen, so the parser has to
 * cope with everything a phone keyboard can produce.
 */
class StudyReminderSchedulerTest {

    @Test
    fun normalisesTwoDigitTimes() {
        assertEquals("20:00", StudyReminderScheduler.normalizeTime("20:00"))
        assertEquals("07:05", StudyReminderScheduler.normalizeTime("07:05"))
        assertEquals("23:59", StudyReminderScheduler.normalizeTime("23:59"))
    }

    @Test
    fun normalisesSingleDigitTimes() {
        assertEquals("07:00", StudyReminderScheduler.normalizeTime("7"))
        assertEquals("07:05", StudyReminderScheduler.normalizeTime("7:5"))
        assertEquals("09:30", StudyReminderScheduler.normalizeTime("9:30"))
    }

    @Test
    fun trimsSurroundingWhitespace() {
        assertEquals("20:00", StudyReminderScheduler.normalizeTime("  20:00  "))
    }

    @Test
    fun acceptsTwelveHourTimes() {
        assertEquals("19:30", StudyReminderScheduler.normalizeTime("7:30 PM"))
        assertEquals("19:30", StudyReminderScheduler.normalizeTime("7:30pm"))
        assertEquals("07:30", StudyReminderScheduler.normalizeTime("7:30 AM"))
        assertEquals("00:00", StudyReminderScheduler.normalizeTime("12:00 AM"))
        assertEquals("12:00", StudyReminderScheduler.normalizeTime("12:00 PM"))
    }

    @Test
    fun rejectsInvalidTimes() {
        assertNull(StudyReminderScheduler.normalizeTime("24:00"))
        assertNull(StudyReminderScheduler.normalizeTime("12:60"))
        assertNull(StudyReminderScheduler.normalizeTime("abc"))
        assertNull(StudyReminderScheduler.normalizeTime(""))
        assertNull(StudyReminderScheduler.normalizeTime("13:00 PM"))
    }
}
