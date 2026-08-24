using System.Drawing;
using CodexTweaks.Windows.Generated;
using H.NotifyIcon;
using H.NotifyIcon.Core;
using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Input;

namespace CodexTweaks.Windows.Services;

internal sealed class TrayIconService : IDisposable
{
    private const int MaximumTooltipLength = 127;
    private const int MaximumBalloonTextLength = 255;

    private readonly MainWindow _window;
    private readonly Func<Task> _quitAsync;
    private readonly DispatcherQueue _dispatcherQueue;
    private readonly MenuFlyout _menu = new();
    private readonly TaskbarIcon _notifyIcon = new();
    private readonly XamlUICommand _showWindowCommand = new();
    private readonly XamlUICommand _refreshMenuCommand = new();
    private readonly MenuFlyoutItem _statusItem = CreateMenuItem(Symbol.Link, false);
    private readonly MenuFlyoutItem _detailItem = new() { IsEnabled = false };
    private readonly MenuFlyoutItem _showItem = CreateMenuItem(Symbol.Home);
    private readonly MenuFlyoutItem _openCodexItem = CreateMenuItem(Symbol.Play);
    private readonly MenuFlyoutItem _restartCodexItem = CreateMenuItem(Symbol.Refresh);
    private readonly ToggleMenuFlyoutItem _enabledItem = new();
    private readonly MenuFlyoutItem _reinjectItem = CreateMenuItem(Symbol.Sync);
    private readonly MenuFlyoutItem _managePackagesItem = CreateMenuItem(Symbol.Library);
    private readonly MenuFlyoutItem _copySkillItem = CreateMenuItem(Symbol.Copy);
    private readonly MenuFlyoutItem _viewLogsItem = CreateMenuItem(Symbol.Document);
    private readonly MenuFlyoutItem _installUpdateItem = CreateMenuItem(Symbol.Download);
    private readonly MenuFlyoutItem _checkUpdatesItem = CreateMenuItem(Symbol.Refresh);
    private readonly MenuFlyoutItem _quitItem = CreateMenuItem(Symbol.Clear);
    private bool _disposed;

    internal TrayIconService(MainWindow window, Func<Task> quitAsync)
    {
        _window = window;
        _quitAsync = quitAsync;
        _dispatcherQueue = window.DispatcherQueue;

        try
        {
            ConfigureMenu();
            ConfigureTrayIcon();
            _window.TrayStateChanged += Window_TrayStateChanged;
            Refresh();
            _notifyIcon.ForceCreate(enablesEfficiencyMode: false);
        }
        catch
        {
            Dispose();
            throw;
        }
    }

    private void ConfigureMenu()
    {
        MenuFlyoutItemBase[] items =
        [
            _statusItem,
            _detailItem,
            new MenuFlyoutSeparator(),
            _showItem,
            _openCodexItem,
            _restartCodexItem,
            _enabledItem,
            _reinjectItem,
            new MenuFlyoutSeparator(),
            _managePackagesItem,
            _copySkillItem,
            _viewLogsItem,
            new MenuFlyoutSeparator(),
            _installUpdateItem,
            _checkUpdatesItem,
            new MenuFlyoutSeparator(),
            _quitItem,
        ];
        foreach (var item in items)
        {
            _menu.Items.Add(item);
        }

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

    private void ConfigureTrayIcon()
    {
        _showWindowCommand.ExecuteRequested += ShowWindowCommand_ExecuteRequested;
        _refreshMenuCommand.ExecuteRequested += RefreshMenuCommand_ExecuteRequested;

        _notifyIcon.Icon = LoadIcon();
        _notifyIcon.ContextMenuMode = ContextMenuMode.SecondWindow;
        _notifyIcon.ContextFlyout = _menu;
        _notifyIcon.LeftClickCommand = _showWindowCommand;
        _notifyIcon.RightClickCommand = _refreshMenuCommand;
        _notifyIcon.NoLeftClickDelay = true;
    }

    private void ShowWindowCommand_ExecuteRequested(
        XamlUICommand sender,
        ExecuteRequestedEventArgs args)
    {
        Dispatch(
            _showItem,
            () =>
            {
                _window.ShowFromTray();
                return Task.CompletedTask;
            });
    }

    private void RefreshMenuCommand_ExecuteRequested(
        XamlUICommand sender,
        ExecuteRequestedEventArgs args)
    {
        Refresh();
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
        _detailItem.Visibility = string.IsNullOrWhiteSpace(statusDetail)
            ? Microsoft.UI.Xaml.Visibility.Collapsed
            : Microsoft.UI.Xaml.Visibility.Visible;
        _showItem.Text = Text(PresentationTextKey.MenuShow);
        _showItem.IsEnabled = true;
        _openCodexItem.Text = Text(PresentationTextKey.OverviewOpenCodex);
        _openCodexItem.IsEnabled = actions?.OpenCodex == true;
        _restartCodexItem.Text = Text(PresentationTextKey.OverviewRestartAndConnect);
        _restartCodexItem.Visibility = actions?.RestartCodex == true
            ? Microsoft.UI.Xaml.Visibility.Visible
            : Microsoft.UI.Xaml.Visibility.Collapsed;
        _restartCodexItem.IsEnabled = actions?.RestartCodex == true;
        _enabledItem.Text = Text(PresentationTextKey.OverviewEnable);
        _enabledItem.IsChecked = snapshot?.Enabled == true;
        _enabledItem.IsEnabled = actions?.SetEnabled == true;
        _reinjectItem.Text = Text(PresentationTextKey.OverviewReinject);
        _reinjectItem.IsEnabled = actions?.Reinject == true;
        _managePackagesItem.Text = Text(PresentationTextKey.OverviewManagePackages);
        _managePackagesItem.IsEnabled = actions?.OpenPackagesDirectory == true;
        _copySkillItem.Text = Text(PresentationTextKey.OverviewCopy);
        _copySkillItem.IsEnabled = actions?.ReadAuthoringPrompt == true;
        _viewLogsItem.Text = Text(PresentationTextKey.OverviewViewLogs);
        _viewLogsItem.IsEnabled = actions?.OpenLogFile == true;

        var updateVersion = _window.AvailableUpdateVersion;
        _installUpdateItem.Text = Text(
            PresentationTextKey.UpdateInstall,
            ("version", updateVersion ?? string.Empty));
        _installUpdateItem.Visibility = string.IsNullOrWhiteSpace(updateVersion)
            ? Microsoft.UI.Xaml.Visibility.Collapsed
            : Microsoft.UI.Xaml.Visibility.Visible;
        _installUpdateItem.IsEnabled = actions?.InstallAppUpdate == true
            && !_window.CheckingUpdate
            && !_window.ApplyingUpdate;
        _checkUpdatesItem.Text = Text(
            _window.CheckingUpdate
                ? PresentationTextKey.UpdateChecking
                : PresentationTextKey.UpdateCheck);
        _checkUpdatesItem.IsEnabled = actions?.CheckAppUpdate == true
            && !_window.CheckingUpdate
            && !_window.ApplyingUpdate;
        _quitItem.Text = Text(PresentationTextKey.MenuQuit);

        _notifyIcon.ToolTipText = Limit(
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
                NotificationIcon.Info);
        }
        else
        {
            ShowError(error);
        }
    }

    private void Dispatch(MenuFlyoutItemBase source, Func<Task> action)
    {
        if (_disposed)
        {
            return;
        }

        source.IsEnabled = false;
        if (!_dispatcherQueue.TryEnqueue(() => _ = ExecuteAsync(action)))
        {
            source.IsEnabled = true;
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
            NotificationIcon.Error);
    }

    private void ShowNotification(string title, string message, NotificationIcon icon)
    {
        if (_disposed)
        {
            return;
        }

        _notifyIcon.ShowNotification(
            Limit(title, MaximumTooltipLength),
            Limit(message, MaximumBalloonTextLength),
            icon,
            timeout: TimeSpan.FromSeconds(3));
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

    private static MenuFlyoutItem CreateMenuItem(Symbol symbol, bool isEnabled = true)
    {
        return new MenuFlyoutItem
        {
            Icon = new SymbolIcon(symbol),
            IsEnabled = isEnabled,
        };
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
        _showWindowCommand.ExecuteRequested -= ShowWindowCommand_ExecuteRequested;
        _refreshMenuCommand.ExecuteRequested -= RefreshMenuCommand_ExecuteRequested;
        _notifyIcon.LeftClickCommand = null;
        _notifyIcon.RightClickCommand = null;
        _notifyIcon.Dispose();
    }
}
