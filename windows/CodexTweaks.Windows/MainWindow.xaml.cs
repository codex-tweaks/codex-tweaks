using System.Diagnostics;
using System.Globalization;
using System.Runtime.InteropServices;
using CodexTweaks.Windows.Generated;
using CodexTweaks.Windows.Models;
using CodexTweaks.Windows.Pages;
using CodexTweaks.Windows.Services;
using Microsoft.UI;
using Microsoft.UI.Windowing;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;
using WinRT.Interop;
using Windows.ApplicationModel.DataTransfer;
using Windows.Graphics;
using Windows.Storage.Pickers;
using Color = global::Windows.UI.Color;

namespace CodexTweaks.Windows;

public sealed partial class MainWindow : Window
{
    private readonly BackendClient _backend = new();
    private readonly VelopackUpdateService _velopack = new();
    private readonly OverviewPage _overviewPage;
    private readonly PackagesPage _packagesPage;
    private readonly LogsPage _logsPage;
    private readonly UpdatesPage _updatesPage;
    private AppWindow? _appWindow;
    private BackendAppSnapshot? _snapshot;
    private VelopackUpdateResult? _velopackResult;
    private string? _trayError;
    private Task? _shutdownTask;
    private string _section = InitialSection();
    private bool _started;
    private bool _applyingUpdate;
    private bool _promptingUpdate;
    private bool _selectingNavigation;
    private bool _trayModeEnabled;
    private bool _allowClose;

    public MainWindow()
    {
        InitializeComponent();
        _overviewPage = new OverviewPage();
        _packagesPage = new PackagesPage();
        _logsPage = new LogsPage();
        _updatesPage = new UpdatesPage();
        ConfigureWindow();
        ApplyStaticText();
        SelectNavigationItem();

        _backend.SnapshotChanged += snapshot =>
            DispatcherQueue.TryEnqueue(() => ApplySnapshot(snapshot));
        _backend.BackendFailed += message =>
            DispatcherQueue.TryEnqueue(() =>
            {
                _trayError = message;
                ShowError(message);
                NotifyTrayStateChanged();
            });
        Activated += MainWindow_Activated;
        Closed += MainWindow_Closed;
    }

    internal event Action? TrayStateChanged;

    internal BackendAppSnapshot? CurrentSnapshot => _snapshot;

    internal string TrayStatusTitle
    {
        get
        {
            var title = _trayError is null
                ? _snapshot?.Presentation.Status.Title
                : Text(PresentationTextKey.StatusErrorTitle);
            return string.IsNullOrWhiteSpace(title)
                ? Text(PresentationTextKey.StatusStartingTitle)
                : title;
        }
    }

    internal string? TrayStatusDetail => _trayError
        ?? _snapshot?.Presentation.Status.Detail;

    internal string? AvailableUpdateVersion =>
        _velopackResult is { Installed: true, Version: { Length: > 0 } version }
            ? version
            : null;

    internal BackendAppSnapshot Snapshot => _snapshot
        ?? throw new InvalidOperationException(
            PresentationFallback.Text(PresentationTextKey.AppBackendNotRunning));

    internal PresentationTokens Tokens =>
        _snapshot?.Presentation.Tokens ?? PresentationDefaults.Tokens;

    internal VelopackUpdateResult? VelopackResult => _velopackResult;

    internal bool ApplyingUpdate => _applyingUpdate;

    private void ConfigureWindow()
    {
        Title = Text(PresentationTextKey.AppName);
        ExtendsContentIntoTitleBar = true;
        SetTitleBar(AppTitleBar);
        SystemBackdrop = new MicaBackdrop();

        try
        {
            var windowHandle = WindowNative.GetWindowHandle(this);
            var windowId = Win32Interop.GetWindowIdFromWindow(windowHandle);
            var appWindow = AppWindow.GetFromWindowId(windowId);
            _appWindow = appWindow;
            var scale = Math.Max(1.0, GetDpiForWindow(windowHandle) / 96.0);
            int Pixels(int dips) => (int)Math.Ceiling(dips * scale);
            if (appWindow.Presenter is OverlappedPresenter presenter)
            {
                presenter.PreferredMinimumWidth = Pixels(Tokens.WindowMinWidth);
                presenter.PreferredMinimumHeight = Pixels(Tokens.WindowMinHeight);
            }
            appWindow.Resize(new SizeInt32(
                Pixels(Tokens.WindowDefaultWidth),
                Pixels(Tokens.WindowDefaultHeight)));
            appWindow.TitleBar.ButtonBackgroundColor = Colors.Transparent;
            appWindow.TitleBar.ButtonInactiveBackgroundColor = Colors.Transparent;
            appWindow.Closing += MainWindow_Closing;
        }
        catch (Exception exception)
        {
            App.Log($"Window chrome configuration fell back to defaults: {exception.Message}");
        }
    }

    private void ApplyStaticText()
    {
        Title = Text(PresentationTextKey.AppName);
        AppTitleText.Text = Text(PresentationTextKey.AppName);
        LoadingText.Text = Text(PresentationTextKey.OverviewConnectingDetail);
        OverviewNavigationItem.Content = Text(PresentationTextKey.NavOverview);
        PackagesNavigationItem.Content = Text(PresentationTextKey.NavPackages);
        LogsNavigationItem.Content = Text(PresentationTextKey.NavLogs);
        UpdatesNavigationItem.Content = Text(PresentationTextKey.NavUpdates);
        ShellNavigation.OpenPaneLength = Tokens.NavigationWidth;
    }

    private async void MainWindow_Activated(object sender, WindowActivatedEventArgs args)
    {
        if (_started)
        {
            return;
        }

        _started = true;
        try
        {
            var snapshot = await _backend.StartAsync();
            ApplySnapshot(snapshot);
            if (snapshot.Update.AutoCheck)
            {
                await CheckVelopackAsync(promptForUpdate: true);
            }
        }
        catch (Exception exception)
        {
            App.Log($"Backend startup failed: {exception}");
            _trayError = exception.Message;
            LoadingPanel.Visibility = Visibility.Collapsed;
            ShowError(exception.Message);
            NotifyTrayStateChanged();
        }
    }

    private void MainWindow_Closing(AppWindow sender, AppWindowClosingEventArgs args)
    {
        if (!_trayModeEnabled || _allowClose)
        {
            return;
        }

        args.Cancel = true;
        sender.Hide();
        App.Log("Main window hidden to the notification area.");
    }

    private void MainWindow_Closed(object sender, WindowEventArgs args)
    {
        _ = ShutdownAsync();
    }

    private void ApplySnapshot(BackendAppSnapshot snapshot)
    {
        if (snapshot.ProtocolVersion != BackendClient.ProtocolVersion)
        {
            ShowError(Text(PresentationTextKey.AppProtocolMismatch));
            return;
        }

        _snapshot = snapshot;
        _trayError = null;
        LoadingPanel.Visibility = Visibility.Collapsed;
        ApplyStaticText();
        RenderCurrentPage();
        NotifyTrayStateChanged();
    }

    internal bool EnableTrayMode()
    {
        if (_appWindow is null)
        {
            return false;
        }

        _trayModeEnabled = true;
        return true;
    }

    internal void ShowFromTray()
    {
        if (_appWindow?.Presenter is OverlappedPresenter
            {
                State: OverlappedPresenterState.Minimized,
            } presenter)
        {
            presenter.Restore();
        }
        if (_appWindow is not null)
        {
            _appWindow.Show(true);
        }
        else
        {
            Activate();
        }
        App.Log("Main window shown from the notification area.");
    }

    internal void CloseForExit()
    {
        _allowClose = true;
        Close();
    }

    internal Task ShutdownAsync()
    {
        return _shutdownTask ??= _backend.DisposeAsync().AsTask();
    }

    private void ShellNavigation_SelectionChanged(
        NavigationView sender,
        NavigationViewSelectionChangedEventArgs args)
    {
        if (_selectingNavigation
            || args.SelectedItemContainer?.Tag is not string section
            || section is not ("overview" or "packages" or "logs" or "updates"))
        {
            return;
        }

        _section = section;
        App.Log($"Navigating to {section}.");
        RenderCurrentPage();
    }

    private void SelectNavigationItem()
    {
        var item = _section switch
        {
            "packages" => PackagesNavigationItem,
            "logs" => LogsNavigationItem,
            "updates" => UpdatesNavigationItem,
            _ => OverviewNavigationItem,
        };
        _selectingNavigation = true;
        ShellNavigation.SelectedItem = item;
        _selectingNavigation = false;
    }

    private void RenderCurrentPage()
    {
        if (_snapshot is null)
        {
            return;
        }

        App.Log($"Rendering {_section} XAML page.");
        Page page = _section switch
        {
            "packages" => _packagesPage,
            "logs" => _logsPage,
            "updates" => _updatesPage,
            _ => _overviewPage,
        };
        PageFrame.Content = page;
        switch (page)
        {
            case OverviewPage overview:
                overview.Render(this, Snapshot);
                break;
            case PackagesPage packages:
                packages.Render(this, Snapshot);
                break;
            case LogsPage logs:
                logs.Render(this, Snapshot);
                break;
            case UpdatesPage updates:
                updates.Render(this, Snapshot);
                break;
        }
    }

    internal string Text(string key, params (string Name, string Value)[] replacements)
    {
        string value;
        if (_snapshot?.Presentation.Text.TryGetValue(key, out var provided) == true)
        {
            value = provided;
        }
        else if (PresentationDefaults.Text.TryGetValue(key, out var fallback))
        {
            value = fallback;
        }
        else
        {
            value = key;
        }

        foreach (var (name, replacement) in replacements)
        {
            value = value.Replace($"{{{name}}}", replacement, StringComparison.Ordinal);
        }
        return value;
    }

    internal Brush ToneBrush(string tone)
    {
        var resourceKey = tone switch
        {
            "success" => "SystemFillColorSuccessBrush",
            "danger" => "SystemFillColorCriticalBrush",
            "warning" => "SystemFillColorCautionBrush",
            "neutral" => "TextFillColorSecondaryBrush",
            _ => "AccentTextFillColorPrimaryBrush",
        };
        if (Application.Current.Resources[resourceKey] is Brush brush)
        {
            return brush;
        }

        var fallback = tone switch
        {
            "success" => Tokens.SuccessColor,
            "danger" => Tokens.DangerColor,
            "warning" => Tokens.WarningColor,
            "neutral" => "#808080",
            _ => Tokens.AccentColor,
        };
        return new SolidColorBrush(ParseColor(fallback));
    }

    internal Brush ToneBackgroundBrush(string tone)
    {
        var color = ParseColor(tone switch
        {
            "success" => Tokens.SuccessColor,
            "danger" => Tokens.DangerColor,
            "warning" => Tokens.WarningColor,
            "neutral" => "#808080",
            _ => Tokens.AccentColor,
        });
        return new SolidColorBrush(Color.FromArgb(30, color.R, color.G, color.B));
    }

    internal async Task<string?> RunBackendAsync(
        string method,
        object? parameters = null,
        bool showError = true)
    {
        try
        {
            await _backend.SendAsync(method, parameters);
            return null;
        }
        catch (Exception exception)
        {
            App.Log($"Backend command {method} failed: {exception}");
            if (showError)
            {
                ShowError(exception.Message);
            }
            return exception.Message;
        }
    }

    internal Task NavigateAsync(string section)
    {
        _section = section;
        SelectNavigationItem();
        RenderCurrentPage();
        return Task.CompletedTask;
    }

    internal static Task OpenPathAsync(string path)
    {
        if (!string.IsNullOrWhiteSpace(path))
        {
            Process.Start(new ProcessStartInfo(path) { UseShellExecute = true });
        }
        return Task.CompletedTask;
    }

    internal async Task PickLocalZipAsync()
    {
        var picker = new FileOpenPicker();
        picker.FileTypeFilter.Add(".zip");
        InitializeWithWindow.Initialize(picker, WindowNative.GetWindowHandle(this));
        var file = await picker.PickSingleFileAsync();
        if (file is not null)
        {
            await RunBackendAsync("installLocalPackage", new { sourcePath = file.Path });
        }
    }

    internal async Task PickLocalDirectoryAsync()
    {
        var picker = new FolderPicker();
        picker.FileTypeFilter.Add("*");
        InitializeWithWindow.Initialize(picker, WindowNative.GetWindowHandle(this));
        var directory = await picker.PickSingleFolderAsync();
        if (directory is not null)
        {
            await RunBackendAsync("installLocalPackage", new { sourcePath = directory.Path });
        }
    }

    internal async Task ShowRemoteInstallDialogAsync()
    {
        var repository = new TextBox
        {
            Header = Text(PresentationTextKey.RemoteRepository),
            PlaceholderText = Text(PresentationTextKey.RemoteRepositoryPlaceholder),
        };
        var selector = new ComboBox
        {
            Header = Text(PresentationTextKey.RemoteSelector),
            HorizontalAlignment = HorizontalAlignment.Stretch,
        };
        var selectorOptions = new (string Value, string Key, bool RequiresValue)[]
        {
            ("branch", PresentationTextKey.SelectorBranch, true),
            ("latestSemverTag", PresentationTextKey.SelectorLatestSemverTag, false),
            ("tag", PresentationTextKey.SelectorTag, true),
            ("githubLatestRelease", PresentationTextKey.SelectorGithubLatestRelease, false),
            ("githubRelease", PresentationTextKey.SelectorGithubRelease, true),
            ("commit", PresentationTextKey.SelectorCommit, true),
        };
        foreach (var option in selectorOptions)
        {
            selector.Items.Add(new ComboBoxItem { Content = Text(option.Key), Tag = option.Value });
        }
        selector.SelectedIndex = 0;
        var selectorValue = new TextBox
        {
            Header = Text(PresentationTextKey.SelectorBranchValue),
            Text = Text(PresentationTextKey.SelectorBranchDefault),
        };
        selector.SelectionChanged += (_, _) =>
        {
            var selected = selectorOptions[selector.SelectedIndex];
            selectorValue.IsEnabled = selected.RequiresValue;
            selectorValue.Header = selected.Value switch
            {
                "tag" => Text(PresentationTextKey.SelectorTagValue),
                "githubRelease" => Text(PresentationTextKey.SelectorGithubReleaseValue),
                "commit" => Text(PresentationTextKey.SelectorCommitValue),
                _ => Text(PresentationTextKey.SelectorBranchValue),
            };
            if (!selected.RequiresValue)
            {
                selectorValue.Text = string.Empty;
            }
        };
        var content = new StackPanel { Spacing = Tokens.ControlSpacing };
        content.Children.Add(repository);
        content.Children.Add(selector);
        content.Children.Add(selectorValue);
        content.Children.Add(new TextBlock
        {
            Text = Text(PresentationTextKey.RemoteValidationDetail),
            TextWrapping = TextWrapping.Wrap,
            Foreground = Application.Current.Resources["TextFillColorSecondaryBrush"] as Brush,
        });
        var dialog = new ContentDialog
        {
            XamlRoot = RootGrid.XamlRoot,
            Title = Text(PresentationTextKey.RemoteTitle),
            Content = content,
            PrimaryButtonText = Text(PresentationTextKey.RemoteInstall),
            CloseButtonText = Text(PresentationTextKey.RemoteClose),
            DefaultButton = ContentDialogButton.Primary,
        };
        if (await dialog.ShowAsync() == ContentDialogResult.Primary)
        {
            var selected = selectorOptions[selector.SelectedIndex];
            await RunBackendAsync("installRemotePackage", new
            {
                repositoryURL = repository.Text.Trim(),
                selectorType = selected.Value,
                selectorValue = selectorValue.Text.Trim(),
            });
        }
    }

    internal async Task ConfirmRestartAsync()
    {
        var dialog = new ContentDialog
        {
            XamlRoot = RootGrid.XamlRoot,
            Title = Text(PresentationTextKey.DialogRestartTitle),
            Content = Text(PresentationTextKey.DialogRestartMessage),
            PrimaryButtonText = Text(PresentationTextKey.OverviewRestartAndConnect),
            CloseButtonText = Text(PresentationTextKey.CommonCancel),
            DefaultButton = ContentDialogButton.Close,
        };
        if (await dialog.ShowAsync() == ContentDialogResult.Primary)
        {
            await RunBackendAsync("restartCodex");
        }
    }

    internal async Task ConfirmClearLogAsync()
    {
        var dialog = new ContentDialog
        {
            XamlRoot = RootGrid.XamlRoot,
            Title = Text(PresentationTextKey.LogsClearTitle),
            Content = Text(PresentationTextKey.LogsClearMessage),
            PrimaryButtonText = Text(PresentationTextKey.LogsClear),
            CloseButtonText = Text(PresentationTextKey.CommonCancel),
            DefaultButton = ContentDialogButton.Close,
        };
        if (await dialog.ShowAsync() == ContentDialogResult.Primary)
        {
            await RunBackendAsync("clearLog");
        }
    }

    internal async Task<string?> CopyAuthoringPromptAsync(bool showFeedback = true)
    {
        try
        {
            var prompt = await _backend.ReadAuthoringPromptAsync();
            var data = new DataPackage();
            data.SetText(prompt);
            Clipboard.SetContent(data);
            if (showFeedback)
            {
                ShowMessage(Text(PresentationTextKey.OverviewCopied), isError: false);
            }
            return null;
        }
        catch (Exception exception)
        {
            App.Log($"Copying the authoring prompt failed: {exception}");
            if (showFeedback)
            {
                ShowError(exception.Message);
            }
            return exception.Message;
        }
    }

    internal async Task CheckUpdatesAsync()
    {
        try
        {
            await _backend.SendAsync("checkAppUpdate", new { prompt = false });
            await CheckVelopackAsync(promptForUpdate: true);
        }
        catch (Exception exception)
        {
            ShowError(Text(PresentationTextKey.UpdateCheckFailed, ("message", exception.Message)));
        }
    }

    internal async Task InstallUpdateAsync()
    {
        if (_applyingUpdate)
        {
            return;
        }

        _applyingUpdate = true;
        RenderCurrentPage();
        NotifyTrayStateChanged();
        var progressBar = new ProgressBar
        {
            Minimum = 0,
            Maximum = 100,
            Value = 0,
            HorizontalAlignment = HorizontalAlignment.Stretch,
        };
        var progressText = new TextBlock
        {
            Text = Text(PresentationTextKey.UpdateDownloadProgress, ("progress", "0")),
            TextWrapping = TextWrapping.Wrap,
        };
        var content = new StackPanel { Spacing = Tokens.ControlSpacing };
        content.Children.Add(progressText);
        content.Children.Add(progressBar);
        var progressDialog = new ContentDialog
        {
            XamlRoot = RootGrid.XamlRoot,
            Title = Text(PresentationTextKey.UpdateApplyProgress),
            Content = content,
        };

        try
        {
            _ = progressDialog.ShowAsync();
            await Task.Yield();
            await _velopack.DownloadAsync(progress =>
                DispatcherQueue.TryEnqueue(() =>
                {
                    var normalized = Math.Clamp(progress, 0, 100);
                    progressBar.Value = normalized;
                    progressText.Text = Text(
                        PresentationTextKey.UpdateDownloadProgress,
                        ("progress", normalized.ToString(CultureInfo.InvariantCulture)));
                }));
            progressBar.Value = 100;
            progressBar.IsIndeterminate = true;
            progressText.Text = Text(PresentationTextKey.UpdateInstallingProgress);
            await Task.Yield();
            _velopack.ApplyAndRestart();
            progressDialog.Hide();
            _applyingUpdate = false;
            RenderCurrentPage();
            NotifyTrayStateChanged();
        }
        catch (Exception exception)
        {
            progressDialog.Hide();
            _applyingUpdate = false;
            ShowError(Text(PresentationTextKey.UpdateInstallFailed, ("message", exception.Message)));
            RenderCurrentPage();
            NotifyTrayStateChanged();
        }
    }

    private async Task CheckVelopackAsync(bool promptForUpdate)
    {
        _velopackResult = await _velopack.CheckAsync(
            Snapshot.Update.Channel,
            Snapshot.Presentation.Platform.Architecture,
            Snapshot.Presentation.Platform.RepositoryURL);
        RenderCurrentPage();
        NotifyTrayStateChanged();
        if (promptForUpdate)
        {
            await PromptForUpdateAsync();
        }
    }

    private async Task PromptForUpdateAsync()
    {
        if (_promptingUpdate
            || _applyingUpdate
            || _velopackResult is not { Installed: true, Version: { Length: > 0 } version })
        {
            return;
        }

        _promptingUpdate = true;
        try
        {
            ShowFromTray();
            var dialog = new ContentDialog
            {
                XamlRoot = RootGrid.XamlRoot,
                Title = Text(PresentationTextKey.UpdateAvailable),
                Content = Text(
                    PresentationTextKey.UpdatePromptMessage,
                    ("current", Snapshot.Update.CurrentVersion),
                    ("latest", version)),
                PrimaryButtonText = Text(PresentationTextKey.UpdateInstall, ("version", version)),
                CloseButtonText = Text(PresentationTextKey.UpdateLater),
                DefaultButton = ContentDialogButton.Close,
            };
            if (await dialog.ShowAsync() == ContentDialogResult.Primary)
            {
                await InstallUpdateAsync();
            }
        }
        finally
        {
            _promptingUpdate = false;
        }
    }

    internal string FormatLastCheck(DateTimeOffset? value)
    {
        return value?.ToLocalTime().ToString("g", CultureInfo.CurrentCulture)
            ?? Text(PresentationTextKey.UpdateNever);
    }

    internal IReadOnlyList<string> PackageMessages(PackageView package)
    {
        var messages = new List<string>();
        AddMessage(package.ValidationError);
        AddMessage(Snapshot.PackageBuildErrors.GetValueOrDefault(package.Id));
        AddMessage(Snapshot.PackageRuntimeErrors.GetValueOrDefault(package.Id));
        AddMessage(Snapshot.PackagePayloadErrors.GetValueOrDefault(package.Id));
        AddMessage(Snapshot.RemotePackageErrors.GetValueOrDefault(package.Id));
        if (Snapshot.PackageDependencyIssues.TryGetValue(package.Id, out var dependencyIssues))
        {
            messages.AddRange(dependencyIssues.Where(value => !string.IsNullOrWhiteSpace(value)));
        }
        return messages;

        void AddMessage(string? value)
        {
            if (!string.IsNullOrWhiteSpace(value))
            {
                messages.Add(value);
            }
        }
    }

    internal void ShowError(string message)
    {
        ShowMessage(message, isError: true);
    }

    internal void ShowMessage(string message, bool isError)
    {
        GlobalInfoBar.Title = isError
            ? Text(PresentationTextKey.StatusErrorTitle)
            : Text(PresentationTextKey.AppName);
        GlobalInfoBar.Message = message;
        GlobalInfoBar.Severity = isError ? InfoBarSeverity.Error : InfoBarSeverity.Success;
        GlobalInfoBar.IsOpen = true;
    }

    private void NotifyTrayStateChanged()
    {
        TrayStateChanged?.Invoke();
    }

    private static Color ParseColor(string value)
    {
        var raw = value.TrimStart('#');
        if (raw.Length == 6 && uint.TryParse(raw, NumberStyles.HexNumber, null, out var rgb))
        {
            return Color.FromArgb(
                255,
                (byte)((rgb >> 16) & 0xff),
                (byte)((rgb >> 8) & 0xff),
                (byte)(rgb & 0xff));
        }
        return Colors.DodgerBlue;
    }

    private static string InitialSection()
    {
        var requested = Environment.GetEnvironmentVariable("CODEX_TWEAKS_INITIAL_SECTION");
        return requested is "packages" or "logs" or "updates" ? requested : "overview";
    }

    [DllImport("user32.dll")]
    private static extern uint GetDpiForWindow(nint windowHandle);
}
