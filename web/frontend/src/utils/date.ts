import { formatInTimeZone } from 'date-fns-tz';
import { enGB } from 'date-fns/locale/en-GB';

/**
 * Get the user's timezone
 */
export function getUserTimezone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone;
}

/**
 * Check if a date is in DST for the user's timezone
 */
function isDST(date: Date): boolean {
  const jan = new Date(date.getFullYear(), 0, 1);
  const jul = new Date(date.getFullYear(), 6, 1);
  const stdOffset = Math.max(jan.getTimezoneOffset(), jul.getTimezoneOffset());
  return date.getTimezoneOffset() < stdOffset;
}

/**
 * Get timezone abbreviation from IANA timezone identifier
 * Returns [standard, daylight] abbreviations
 */
function getTimezoneAbbr(timezone: string, date: Date): string {
  const dst = isDST(date);

  // Map IANA timezone to [standard, daylight] abbreviations
  const tzMap: Record<string, [string, string]> = {
    // Central European Time
    'Europe/Rome': ['CET', 'CEST'],
    'Europe/Paris': ['CET', 'CEST'],
    'Europe/Berlin': ['CET', 'CEST'],
    'Europe/Madrid': ['CET', 'CEST'],
    'Europe/Amsterdam': ['CET', 'CEST'],
    'Europe/Brussels': ['CET', 'CEST'],
    'Europe/Vienna': ['CET', 'CEST'],
    'Europe/Warsaw': ['CET', 'CEST'],
    'Europe/Prague': ['CET', 'CEST'],
    'Europe/Budapest': ['CET', 'CEST'],
    'Europe/Stockholm': ['CET', 'CEST'],
    'Europe/Oslo': ['CET', 'CEST'],
    'Europe/Copenhagen': ['CET', 'CEST'],
    'Europe/Zurich': ['CET', 'CEST'],
    // UK/Ireland
    'Europe/London': ['GMT', 'BST'],
    'Europe/Dublin': ['GMT', 'IST'],
    // Western European
    'Europe/Lisbon': ['WET', 'WEST'],
    // Eastern European
    'Europe/Athens': ['EET', 'EEST'],
    'Europe/Helsinki': ['EET', 'EEST'],
    'Europe/Bucharest': ['EET', 'EEST'],
    'Europe/Sofia': ['EET', 'EEST'],
    'Europe/Kiev': ['EET', 'EEST'],
    'Europe/Kyiv': ['EET', 'EEST'],
    // Russia
    'Europe/Moscow': ['MSK', 'MSK'],
    // US
    'America/New_York': ['EST', 'EDT'],
    'America/Chicago': ['CST', 'CDT'],
    'America/Denver': ['MST', 'MDT'],
    'America/Los_Angeles': ['PST', 'PDT'],
    'America/Phoenix': ['MST', 'MST'],
    'America/Toronto': ['EST', 'EDT'],
    'America/Vancouver': ['PST', 'PDT'],
    // Asia/Pacific
    'Asia/Tokyo': ['JST', 'JST'],
    'Asia/Shanghai': ['CST', 'CST'],
    'Asia/Hong_Kong': ['HKT', 'HKT'],
    'Asia/Singapore': ['SGT', 'SGT'],
    'Asia/Dubai': ['GST', 'GST'],
    'Australia/Sydney': ['AEST', 'AEDT'],
    'Australia/Melbourne': ['AEST', 'AEDT'],
    'Australia/Perth': ['AWST', 'AWST'],
    'Pacific/Auckland': ['NZST', 'NZDT'],
  };

  const abbrs = tzMap[timezone];
  if (abbrs) {
    return dst ? abbrs[1] : abbrs[0];
  }

  // Fallback: return the timezone identifier
  return timezone.split('/').pop() || timezone;
}

/**
 * Format a timestamp with timezone for display in the UI
 * Format: "5 Jan 2026, 18:16:42 CET"
 */
export function formatDateTime(timestamp: string | Date): string {
  const date = typeof timestamp === 'string' ? new Date(timestamp) : timestamp;
  const timezone = getUserTimezone();
  const tzAbbr = getTimezoneAbbr(timezone, date);

  return `${formatInTimeZone(date, timezone, 'd MMM yyyy, HH:mm:ss', { locale: enGB })} ${tzAbbr}`;
}

/**
 * Format a timestamp for compact display (e.g., in cards)
 * Format: "5 Jan, 18:16"
 */
export function formatDateTimeShort(timestamp: string | Date): string {
  const date = typeof timestamp === 'string' ? new Date(timestamp) : timestamp;
  const timezone = getUserTimezone();

  return formatInTimeZone(date, timezone, 'd MMM, HH:mm', { locale: enGB });
}

/**
 * Format just the time portion
 * Format: "18:16:42"
 */
export function formatTime(timestamp: string | Date): string {
  const date = typeof timestamp === 'string' ? new Date(timestamp) : timestamp;
  const timezone = getUserTimezone();

  return formatInTimeZone(date, timezone, 'HH:mm:ss', { locale: enGB });
}

/**
 * Format just the date portion
 * Format: "5 Jan 2026"
 */
export function formatDate(timestamp: string | Date): string {
  const date = typeof timestamp === 'string' ? new Date(timestamp) : timestamp;
  const timezone = getUserTimezone();

  return formatInTimeZone(date, timezone, 'd MMM yyyy', { locale: enGB });
}
