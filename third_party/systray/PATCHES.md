# systray v0.3.0

Source: github.com/gogpu/systray v0.3.0, MIT licence retained in LICENSE.

The Windows Show and modifyIcon calls include the existing 0x80 flag (upstream calls it nifShovel). Windows documents this as NIF_SHOWTIP, required for standard hover tooltips with NOTIFYICON_VERSION_4. Both paths are patched so taskbar recovery and theme changes retain the tooltip. All other upstream code is unchanged.

Reference: https://learn.microsoft.com/en-us/windows/win32/api/shellapi/ns-shellapi-notifyicondataw

Remove the local replacement when an upstream release includes this fix.
