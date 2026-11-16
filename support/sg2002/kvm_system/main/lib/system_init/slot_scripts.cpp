#include "slot_scripts.h"

#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>

int slot_refuses(const char *list, const char *name)
{
	FILE *fp = fopen(list, "r");
	if (fp == NULL) return 0;

	char line[256];
	int found = 0;
	while (!found && fgets(line, sizeof(line), fp) != NULL) {
		char *s = line;
		while (*s == ' ' || *s == '\t') s++;
		if (*s == '#') continue;
		size_t n = strlen(s);
		while (n > 0 && (s[n - 1] == '\n' || s[n - 1] == '\r' ||
				 s[n - 1] == ' ' || s[n - 1] == '\t'))
			s[--n] = '\0';
		if (n > 0 && strcmp(s, name) == 0) found = 1;
	}
	fclose(fp);
	return found;
}

// Writes all of buf, or answers -1.
static int write_all(int fd, const char *buf, size_t len)
{
	while (len > 0) {
		ssize_t n = write(fd, buf, len);
		if (n < 0) {
			if (errno == EINTR) continue;
			return -1;
		}
		buf += n;
		len -= (size_t)n;
	}
	return 0;
}

int copy_boot_script(const char *src_dir, const char *dest_dir, const char *name)
{
	char src[512], tmp[512], dest[512];
	if (snprintf(src, sizeof(src), "%s/%s", src_dir, name) >= (int)sizeof(src) ||
	    snprintf(dest, sizeof(dest), "%s/%s", dest_dir, name) >= (int)sizeof(dest) ||
	    snprintf(tmp, sizeof(tmp), "%s/.%s.new", dest_dir, name) >= (int)sizeof(tmp)) {
		printf("new_app_init: %s: path too long, not installed\n", name);
		return -1;
	}

	int in = open(src, O_RDONLY | O_CLOEXEC);
	if (in < 0) {
		printf("new_app_init: %s: %s, not installed\n", src, strerror(errno));
		return -1;
	}
	struct stat st;
	if (fstat(in, &st) != 0) {
		printf("new_app_init: %s: %s, not installed\n", src, strerror(errno));
		close(in);
		return -1;
	}

	// A copy that died earlier may have left this name behind.
	unlink(tmp);
	int out = open(tmp, O_WRONLY | O_CREAT | O_EXCL | O_CLOEXEC, 0600);
	if (out < 0) {
		printf("new_app_init: %s: %s, not installed\n", tmp, strerror(errno));
		close(in);
		return -1;
	}

	int ok = 1;
	char buf[4096];
	for (;;) {
		ssize_t n = read(in, buf, sizeof(buf));
		if (n == 0) break;
		if (n < 0) {
			if (errno == EINTR) continue;
			ok = 0;
			break;
		}
		if (write_all(out, buf, (size_t)n) != 0) {
			ok = 0;
			break;
		}
	}
	// The mode last, so the name never exists as something rcS could run
	// half written. The data is on the card before the rename makes it the
	// script: a rename that survives a power cut while its data does not
	// would leave an empty boot script.
	if (ok && fchmod(out, st.st_mode & 07777) != 0) ok = 0;
	if (ok && fsync(out) != 0) ok = 0;
	int saved = errno;
	close(in);
	if (close(out) != 0 && ok) {
		ok = 0;
		saved = errno;
	}
	if (ok && rename(tmp, dest) != 0) {
		ok = 0;
		saved = errno;
	}
	if (!ok) {
		printf("new_app_init: %s: %s, not installed\n", dest, strerror(saved));
		unlink(tmp);
		return -1;
	}

	int dir = open(dest_dir, O_RDONLY | O_DIRECTORY | O_CLOEXEC);
	if (dir >= 0) {
		fsync(dir);
		close(dir);
	}
	return 0;
}

int install_boot_script(const char *list, const char *src_dir,
			const char *dest_dir, const char *name)
{
	if (slot_refuses(list, name)) {
		printf("new_app_init: %s is refused by this slot (%s), not installed\n",
		       name, list);
		return 0;
	}
	copy_boot_script(src_dir, dest_dir, name);
	return 1;
}

enum s30wifi_action s30wifi_action(const char *list, int vendor, int wifi)
{
	if (slot_refuses(list, "S30wifi")) return S30WIFI_REMOVE;
	if (vendor && wifi) return S30WIFI_INSTALL;
	if (access(list, F_OK) == 0) return S30WIFI_KEEP;
	return S30WIFI_REMOVE;
}
