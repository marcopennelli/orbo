import { format } from 'date-fns';
import { toZonedTime } from 'date-fns-tz';

/**
 * Get the user's timezone
 */
export function getUserTimezone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone;
}

/**
 * Get timezone abbreviation (e.g., CET, EST, PST)
 */
function getTimezoneAbbreviation(date: Date, timezone: string): string {
  const formatter = new Intl.DateTimeFormat('en-US', {
    timeZone: timezone,
    timeZoneName: 'short',
  });
  const parts = formatter.formatToParts(date);
  const tzPart = parts.find(p => p.type === 'timeZoneName');
  return tzPart?.value || timezone;
}

/**
 * Format a timestamp with timezone for display in the UI
 * Format: "5 Jan 2026, 18:16:42 CET"
 */
export function formatDateTime(timestamp: string | Date): string {
  const date = typeof timestamp === 'string' ? new Date(timestamp) : timestamp;
  const timezone = getUserTimezone();
  const zonedDate = toZonedTime(date, timezone);
  const tzAbbr = getTimezoneAbbreviation(date, timezone);

  return `${format(zonedDate, 'd MMM yyyy, HH:mm:ss')} ${tzAbbr}`;
}

/**
 * Format a timestamp for compact display (e.g., in cards)
 * Format: "5 Jan, 18:16"
 */
export function formatDateTimeShort(timestamp: string | Date): string {
  const date = typeof timestamp === 'string' ? new Date(timestamp) : timestamp;
  const timezone = getUserTimezone();
  const zonedDate = toZonedTime(date, timezone);

  return format(zonedDate, 'd MMM, HH:mm');
}

/**
 * Format just the time portion
 * Format: "18:16:42"
 */
export function formatTime(timestamp: string | Date): string {
  const date = typeof timestamp === 'string' ? new Date(timestamp) : timestamp;
  const timezone = getUserTimezone();
  const zonedDate = toZonedTime(date, timezone);

  return format(zonedDate, 'HH:mm:ss');
}

/**
 * Format just the date portion
 * Format: "5 Jan 2026"
 */
export function formatDate(timestamp: string | Date): string {
  const date = typeof timestamp === 'string' ? new Date(timestamp) : timestamp;
  const timezone = getUserTimezone();
  const zonedDate = toZonedTime(date, timezone);

  return format(zonedDate, 'd MMM yyyy');
}
