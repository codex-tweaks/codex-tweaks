using System.ComponentModel;
using System.Drawing;
using CodexTweaks.Windows.Generated;
using Microsoft.UI.Dispatching;
using Forms = System.Windows.Forms;

namespace CodexTweaks.Windows.Services;

internal sealed class TrayIconService : IDisposable
{
    private const int MaximumTooltipLength = 127;
    private const int MaximumBalloonTextLength = 255;

    private readonly MainWindow _window;
    private readonly Func<Task> _quitAsync;
    private readonly DispatcherQueue _dispatcherQueue;
    private readonly Forms.ContextMenuStrip _menu = new()
    {
        ShowCheckMargin = true,
        ShowImageMargin = false,
    };
    private readonly Forms.NotifyIcon _notifyIcon = new();
    private readonly Icon _icon;
    private readonly Forms.ToolStripMenuItem _statusItem = new() { Enabled = false };
    private readonly Forms.ToolStripMenuItem _detailItem = new() { Enabled = false };
    private readonly Forms.ToolStripMenuItem _showItem = new();
    private readonly Forms.ToolStripMenuItem _openCodexItem = new();
    private readonly Forms.ToolStripMenuItem _restartCodexItem = new();
    private readonly Forms.ToolStripMenuItem _enabledItem = new() { CheckOnClick = false };
    private readonly Forms.ToolStripMenuItem _reinjectItem = new();
    private readonly Forms.ToolStripMenuItem _managePackagesItem = new();
    private readonly Forms.ToolStripMenuItem _copySkillItem = new();
    private readonly Forms.ToolStripMenuItem _viewLogsItem = new();
    private readonly Forms.ToolStripMenuItem _installUpdateItem = new();
    private readonly Forms.ToolStripMenuItem _checkUpdatesItem = new();
    private readonly Forms.ToolStripMenuItem _quitItem = new();
    private bool _disposed;

    internal TrayIconService(MainWindow window, Func<Task> quitAsync)
    {
        _window = window;
        _quitAsync = quitAsync;
        _dispatcherQueue = window.DispatcherQueue;
        _icon = LoadIcon();

        ConfigureMenu();
        _menu.Opening += Menu_Opening;
        _notifyIcon.Icon = _icon;
        _notifyIcon.ContextMenuStrip = _menu;
        _notifyIcon.MouseDoubleClick += NotifyIcon_MouseDoubleClick;
        _window.TrayStateChanged += Window_TrayStateChanged;
        Refresh();
        _notifyIcon.Visible = true;
    }

    private void ConfigureMenu()
    {
        _menu.Items.AddRange(
        [
            _statusItem,
            _detailItem,
            new Forms.ToolStripSeparator(),
            _showItem,
            _openCodexItem,
            _restartCodexItem,
            _enabledItem,
            _reinjectItem,
            new Forms.ToolStripSeparator(),
            _managePackagesItem,
            _copySkillItem,
            _viewLogsItem,
            new Forms.ToolStripSeparator(),
            _installUpdateItem,
            _checkUpdatesItem,
            new Forms.ToolStripSeparator(),
            _quitItem,
        ]);

        _showItem.Click += (_, _) => Dispatch(
            _showItem,
            () =>
            {
                _window.ShowFromTray();
                return Task.CompletedTask;
            });
        _openCodexItem.Click += (_, _) => Dispatch(
            _openCodexItem,
            () => RunBackendFromTrayAsync("openCodex"));
        _restartCodexItem.Click += (_, _) => Dispatch(
            _restartCodexItem,
            async () =>
            {
                _window.ShowFromTray();
                await _window.ConfirmRestartAsync();
            });
        _enabledItem.Click += (_, _) =>
        {
            var snapshot = _window.CurrentSnapshot;
            if (snapshot is not null)
            {
                Dispatch(
                    _enabledItem,
                    () => RunBackendFromTrayAsync(
                        "setEnabled",
                        new { enabled = !snapshot.Enabled }));
            }
        };
        _reinjectItem.Click += (_, _) => Dispatch(
            _reinjectItem,
            () => RunBackendFromTrayAsync("reinject"));
        _managePackagesItem.Click += (_, _) => Dispatch(
            _managePackagesItem,
            () => MainWindow.OpenPathAsync(
                _window.CurrentSnapshot?.PackagesDirectory ?? string.Empty));
        _copySkillItem.Click += (_, _) => Dispatch(
            _copySkillItem,
            CopySkillFromTrayAsync);
        _viewLogsItem.Click += (_, _) => Dispatch(
            _viewLogsItem,
            () => MainWindow.OpenPathAsync(
                _window.CurrentSnapshot?.LogPath ?? string.Empty));
        _installUpdateItem.Click += (_, _) => Dispatch(
            _installUpdateItem,
            async () =>
            {
                _window.ShowFromTray();
                await _window.InstallUpdateAsync();
            });
        _checkUpdatesItem.Click += (_, _) => Dispatch(
            _checkUpdatesItem,
            async () =>
            {
                _window.ShowFromTray();
                await _window.CheckUpdatesAsync();
            });
        _quitItem.Click += (_, _) => Dispatch(_quitItem, _quitAsync);
    }

    private void Menu_Opening(object? sender, CancelEventArgs args)
    {
        Refresh();
    }

    private void NotifyIcon_MouseDoubleClick(object? sender, Forms.MouseEventArgs args)
    {
        if (args.Button == Forms.MouseButtons.Left)
        {
            Dispatch(
                _showItem,
                () =>
                {
                    _window.ShowFromTray();
                    return Task.CompletedTask;
                });
        }
    }

    private void Window_TrayStateChanged()
    {
        Refresh();
    }

    private void Refresh()
    {
        if (_disposed)
        {
            return;
        }

        var snapshot = _window.CurrentSnapshot;
        var actions = snapshot?.Presentation.Actions;
        var statusTitle = _window.TrayStatusTitle;
        var statusDetail = _window.TrayStatusDetail?.Trim();

        _statusItem.Text = Limit(statusTitle, 96);
        _detailItem.Text = Limit(statusDetail ?? string.Empty, 120);
        _detailItem.Visible = !string.IsNullOrWhiteSpace(statusDetail);
        _showItem.Text = Text(PresentationTextKey.MenuShow);
        _openCodexItem.Text = Text(PresentationTextKey.OverviewOpenCodex);
        _openCodexItem.Enabled = actions?.OpenCodex == true;
        _restartCodexItem.Text = Text(PresentationTextKey.OverviewRestartAndConnect);
        _restartCodexItem.Visible = actions?.RestartCodex == true;
        _restartCodexItem.Enabled = actions?.RestartCodex == true;
        _enabledItem.Text = Text(PresentationTextKey.OverviewEnable);
        _enabledItem.Checked = snapshot?.Enabled == true;
        _enabledItem.Enabled = actions?.SetEnabled == true;
        _reinjectItem.Text = Text(PresentationTextKey.OverviewReinject);
        _reinjectItem.Enabled = actions?.Reinject == true;
        _managePackagesItem.Text = Text(PresentationTextKey.OverviewManagePackages);
        _managePackagesItem.Enabled = actions?.OpenPackagesDirectory == true;
        _copySkillItem.Text = Text(PresentationTextKey.OverviewCopy);
        _copySkillItem.Enabled = actions?.ReadAuthoringPrompt == true;
        _viewLogsItem.Text = Text(PresentationTextKey.OverviewViewLogs);
        _viewLogsItem.Enabled = actions?.OpenLogFile == true;

        var updateVersion = _window.AvailableUpdateVersion;
        _installUpdateItem.Text = Text(
            PresentationTextKey.UpdateInstall,
            ("version", updateVersion ?? string.Empty));
        _installUpdateItem.Visible = !string.IsNullOrWhiteSpace(updateVersion);
        _installUpdateItem.Enabled = actions?.InstallAppUpdate == true
            && !_window.CheckingUpdate
            && !_window.ApplyingUpdate;
        _checkUpdatesItem.Text = Text(
            _window.CheckingUpdate
                ? PresentationTextKey.UpdateChecking
                : PresentationTextKey.UpdateCheck);
        _checkUpdatesItem.Enabled = actions?.CheckAppUpdate == true
            && !_window.CheckingUpdate
            && !_window.ApplyingUpdate;
        _quitItem.Text = Text(PresentationTextKey.MenuQuit);

        _notifyIcon.Text = Limit(
            $"{Text(PresentationTextKey.AppName)} — {statusTitle}",
            MaximumTooltipLength);
    }

    private async Task RunBackendFromTrayAsync(string method, object? parameters = null)
    {
        var error = await _window.RunBackendAsync(method, parameters, showError: false);
        if (!string.IsNullOrWhiteSpace(error))
        {
            ShowError(error);
        }
    }

    private async Task CopySkillFromTrayAsync()
    {
        var error = await _window.CopyAuthoringPromptAsync(showFeedback: false);
        if (string.IsNullOrWhiteSpace(error))
        {
            ShowNotification(
                Text(PresentationTextKey.AppName),
                Text(PresentationTextKey.OverviewCopied),
                Forms.ToolTipIcon.Info);
        }
        else
        {
            ShowError(error);
        }
    }

    private void Dispatch(Forms.ToolStripItem source, Func<Task> action)
    {
        if (_disposed)
        {
            return;
        }

        source.Enabled = false;
        if (!_dispatcherQueue.TryEnqueue(() => _ = ExecuteAsync(action)))
        {
            source.Enabled = true;
        }
    }

    private async Task ExecuteAsync(Func<Task> action)
    {
        try
        {
            await action();
        }
        catch (Exception exception)
        {
            App.Log($"Tray command failed: {exception}");
            ShowError(exception.Message);
        }
        finally
        {
            Refresh();
        }
    }

    private void ShowError(string message)
    {
        ShowNotification(
            Text(PresentationTextKey.StatusErrorTitle),
            message,
            Forms.ToolTipIcon.Error);
    }

    private void ShowNotification(string title, string message, Forms.ToolTipIcon icon)
    {
        if (_disposed)
        {
            return;
        }

        _notifyIcon.ShowBalloonTip(
            3000,
            Limit(title, MaximumTooltipLength),
            Limit(message, MaximumBalloonTextLength),
            icon);
    }

    private string Text(string key, params (string Name, string Value)[] replacements)
    {
        return _window.Text(key, replacements);
    }

    private static string Limit(string value, int maximumLength)
    {
        return value.Length <= maximumLength
            ? value
            : string.Concat(value.AsSpan(0, maximumLength - 1), "…");
    }

    private static Icon LoadIcon()
    {
        var assetPath = Path.Combine(AppContext.BaseDirectory, "Assets", "CodexTweaks.ico");
        if (File.Exists(assetPath))
        {
            return new Icon(assetPath);
        }

        if (Environment.ProcessPath is { Length: > 0 } executablePath
            && Icon.ExtractAssociatedIcon(executablePath) is { } executableIcon)
        {
            return executableIcon;
        }

        return (Icon)SystemIcons.Application.Clone();
    }

    public void Dispose()
    {
        if (_disposed)
        {
            return;
        }

        _disposed = true;
        _window.TrayStateChanged -= Window_TrayStateChanged;
        _menu.Opening -= Menu_Opening;
        _notifyIcon.MouseDoubleClick -= NotifyIcon_MouseDoubleClick;
        _notifyIcon.Visible = false;
        _notifyIcon.ContextMenuStrip = null;
        _notifyIcon.Dispose();
        _menu.Dispose();
        _icon.Dispose();
    }
}
