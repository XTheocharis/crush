package stdlib

import "strings"

var phpClasses = toSet([]string{
	"ArrayAccess", "ArrayObject", "AssertionError", "AssertionError|AssertionError",
	"BackedEnum", "Closure", "DateTime", "DateTimeImmutable",
	"DateTimeZone", "DateInterval", "DatePeriod", "Directory",
	"DirectoryIterator", "DomainException", "Exception",
	"FilterIterator", "FilesystemIterator", "Generator",
	"GlobIterator", "InfiniteIterator", "IntlCalendar",
	"IntlDateFormatter", "IntlException", "IntlIterator",
	"InvalidArgumentException", "Iterator", "IteratorIterator",
	"JsonSerializable", "LengthException", "LimitIterator",
	"LogicException", "MultipleIterator", "NoRewindIterator",
	"OutOfBoundsException", "OutOfRangeException", "OverflowException",
	"ParentIterator", "RangeException", "RecursiveArrayIterator",
	"RecursiveDirectoryIterator", "RecursiveIteratorIterator",
	"ReflectionClass", "ReflectionException", "ReflectionFunction",
	"ReflectionMethod", "ReflectionObject", "ReflectionParameter",
	"ReflectionProperty", "ReflectionExtension", "Reflector",
	"RegexIterator", "RuntimeException", "SeekableIterator",
	"Serializable", "SplFileInfo", "SplFileObject",
	"SplFixedArray", "SplHeap", "SplMaxHeap", "SplMinHeap",
	"SplObjectStorage", "SplObserver", "SplPriorityQueue",
	"SplQueue", "SplStack", "SplSubject", "SplTempFileObject",
	"stdClass", "Stringable", "Throwable",
	"UnderflowException", "UnexpectedValueException", "UnitEnum",
	"UnhandledMatchError", "ValueError", "WeakMap", "WeakReference",
	"__PHP_Incomplete_Class",
})

var phpFunctions = toSet([]string{
	"array_change_key_case", "array_chunk", "array_combine", "array_count_values",
	"array_diff", "array_diff_assoc", "array_diff_key", "array_fill", "array_filter",
	"array_flip", "array_key_exists", "array_keys", "array_map", "array_merge",
	"array_multisort", "array_pad", "array_pop", "array_push", "array_rand",
	"array_reduce", "array_reverse", "array_search", "array_shift", "array_slice",
	"array_sum", "array_unique", "array_unshift", "array_values", "array_walk",
	"arsort", "asort", "count", "in_array", "is_array", "json_decode", "json_encode",
	"ksort", "natcasesort", "natsort", "rsort", "shuffle", "sort", "uasort",
	"uksort", "usort", "compact", "extract", "get_defined_vars", "isset", "print_r",
	"var_dump", "var_export", "chr", "count_chars", "explode", "implode", "join",
	"lcfirst", "levenshtein", "localeconv", "ltrim", "md5", "metaphone", "money_format",
	"nl_langinfo", "nl2br", "number_format", "ord", "parse_str", "print", "printf",
	"quoted_printable_decode", "quoted_printable_encode", "quotemeta", "rtrim", "setlocale",
	"sha1", "similar_text", "soundex", "sprintf", "sscanf", "str_contains",
	"str_ends_with", "str_getcsv", "str_ireplace", "str_pad", "str_repeat",
	"str_replace", "str_rot13", "str_shuffle", "str_split", "str_starts_with",
	"strcasecmp", "strchr", "strcmp", "strcoll", "strcspn", "stripos", "stripslashes",
	"stristr", "strlen", "strnatcasecmp", "strnatcmp", "strncasecmp", "strncmp",
	"strpbrk", "strpos", "strrchr", "strrev", "strripos", "strrpos", "strspn",
	"strstr", "strtok", "strtolower", "strtoupper", "strtr", "substr_compare",
	"substr_count", "substr_replace", "trim", "ucfirst", "ucwords", "wordwrap",
	"abs", "acos", "acosh", "asin", "asinh", "atan2", "atan", "atanh", "base_convert",
	"bindec", "ceil", "cos", "cosh", "decbin", "dechex", "decoct", "deg2rad",
	"exp", "expm1", "fdiv", "floor", "fmod", "getrandmax", "hexdec", "hypot",
	"intdiv", "is_finite", "is_infinite", "is_nan", "log10", "log1p", "log",
	"max", "min", "mt_getrandmax", "mt_rand", "octdec", "pi", "pow",
	"rad2deg", "rand", "round", "sin", "sinh", "sqrt", "tan", "tanh",
	"base64_decode", "base64_encode", "crypt", "hash", "hash_equals",
	"hash_hmac", "hash_hmac_algos", "md5_file", "sha1_file", "password_hash",
	"password_needs_rehash", "password_verify", "checkdate", "date_add",
	"date_create", "date_format", "date_parse", "getdate", "gettimeofday",
	"gmdate", "gmmktime", "gmstrftime", "localtime", "microtime", "mktime",
	"strftime", "strtotime", "time", "chdir", "chroot", "closedir", "dir",
	"getcwd", "opendir", "readdir", "rewinddir", "scandir", "dirname",
	"pathinfo", "realpath", "basename", "copy", "filesize", "file_exists",
	"file_get_contents", "file_put_contents", "filemtime", "fileperms", "is_dir",
	"is_executable", "is_file", "is_link", "is_readable", "is_writable",
	"mkdir", "readlink", "rename", "rmdir", "stat", "symlink", "tempnam",
	"tmpfile", "touch", "unlink", "filesize", "clearstatcache", "disk_free_space",
	"disk_total_space", "fileinode", "fileowner", "filegroup", "fileatime",
	"filectime", "filetype", "fstat", "lstat", "chmod", "chown", "chgrp",
	"lchown", "umask", "basename", "clearstatcache", "copy", "dirname",
	"disk_free_space", "disk_total_space", "fclose", "feof", "fflush", "fgetc",
	"fgetcsv", "fgets", "fgetss", "file_exists", "file_get_contents", "file_put_contents",
	"file", "fileatime", "filectime", "filegroup", "fileinode", "filemtime",
	"fileowner", "fileperms", "filesize", "filetype", "flock", "fnmatch",
	"fopen", "fpassthru", "fputcsv", "fputs", "fread", "fscanf", "fseek",
	"fstat", "ftell", "ftruncate", "fwrite", "glob", "is_dir", "is_executable",
	"is_file", "is_link", "is_readable", "is_uploaded_file", "is_writable",
	"is_writeable", "lchgrp", "lchown", "link", "linkinfo", "lstat", "mkdir",
	"move_uploaded_file", "parse_ini_file", "parse_ini_string", "pathinfo",
	"pclose", "popen", "readfile", "readlink", "realpath_cache_get",
	"realpath_cache_size", "realpath", "rename", "rewind", "rmdir", "set_file_buffer",
	"stat", "symlink", "tempnam", "tmpfile", "touch", "umask", "unlink",
	"defined", "die", "empty", "eval", "exit", "include", "include_once",
	"isset", "list", "require", "require_once", "trigger_error", "unset", "user_error",
	"debug_backtrace", "debug_print_backtrace", "error_get_last", "error_log",
	"error_reporting", "restore_error_handler", "restore_exception_handler",
	"set_error_handler", "set_exception_handler", "set_time_limit", "sleep",
	"time_nanosleep", "time_sleep_until", "usleep", "connection_aborted",
	"connection_status", "constant", "define", "defined", "get_cfg_var",
	"get_current_user", "get_defined_constants", "get_defined_functions",
	"get_extension_funcs", "get_included_files", "get_required_files",
	"get_loaded_extensions", "get_magic_quotes_gpc", "get_magic_quotes_runtime",
	"getcwd", "hostname_gethostname", "ini_alter", "ini_get_all", "ini_get",
	"ini_restore", "ini_set", "memory_get_peak_usage", "memory_get_usage",
	"php_ini_loaded_file", "php_ini_scanned_files", "php_sapi_name",
	"php_uname", "phpinfo", "phpversion", "putenv", "set_include_path",
	"set_magic_quotes_runtime", "set_time_limit", "version_compare", "zend_version",
	"header", "header_remove", "headers_list", "headers_sent", "setcookie",
	"setrawcookie", "filter_has_var", "filter_id", "filter_input_array",
	"filter_input", "filter_list", "filter_var_array", "filter_var", "http_response_code",
	"gethostbyaddr", "gethostbyname", "gethostbynamel", "gethostname",
	"getprotobyname", "getprotobynumber", "ip2long", "long2ip",
	"checkdnsrr", "getmxrr", "dns_check_record", "dns_get_mx",
	"sys_getloadavg", "getmypid", "getmyuid", "getmygid", "phpcredits",
	"fsockopen", "pfsockopen", "gettimeofday", "getrusage",
})

func IsPHPStdlib(symbol string) bool {
	symbol = strings.TrimSpace(symbol)
	if phpClasses[symbol] {
		return true
	}
	if phpFunctions[symbol] {
		return true
	}
	if strings.HasPrefix(symbol, "DateTime") {
		return true
	}
	if strings.HasPrefix(symbol, "Intl") {
		return true
	}
	if strings.HasPrefix(symbol, "Reflector") {
		return true
	}
	return false
}
