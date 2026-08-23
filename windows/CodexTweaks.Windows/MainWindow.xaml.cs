using System.Diagnostics;
using System.Globalization;
using CodexTweaks.Windows.Generated;
using CodexTweaks.Windows.Models;
using CodexTweaks.Windows.Services;
using Microsoft.UI;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Xaml.Media;
using WinRT.Interop;
using Windows.ApplicationModel.DataTransfer;
using Windows.Storage.Pickers;
using Windows.System;
using Color = global::Windows.UI.Color;

namespace CodexTweaks.Windows;

public sealed class MainWindow : Window
{
    private readonly BackendClient _backend = new();
    private readonly VelopackUpdateService _velopack = new();
    private readonly Button OverviewButton = new();
    private readonly Button PackagesButton = new();
    private readonly Button LogsButton = new();
    private readonly Button UpdatesButton = new();
    private readonly ColumnDefinition SidebarColumn = new();
    private readonly Grid PageHost = new() { HorizontalAlignment = HorizontalAlignment.Stretch };
    private readonly TextBlock GlobalMessageText = new() { TextWrapping = TextWrapping.Wrap };
    private readonly Border GlobalMessage = new()
    {
        Margin = new Thickness(16),
        Padding = new Thickness(14, 10, 14, 10),
        CornerRadius = new CornerRadius(8),
        HorizontalAlignment = HorizontalAlignment.Stretch,
        VerticalAlignment = VerticalAlignment.Top,
        Visibility = Visibility.Collapsed,
    };
    private readonly Grid RootGrid = new();
    private BackendAppSnapshot? _snapshot;
    private VelopackUpdateResult? _velopackResult;
    private string _section = InitialSection();
    private string _packageSearch = string.Empty;
    private bool _started;
    private bool _applyingUpdate;

    public MainWindow()
    {
        Content = RootGrid;
        BuildShell();
        ApplyChrome();
        RenderLoading();

        _backend.SnapshotChanged += snapshot =>
            DispatcherQueue.TryEnqueue(() => ApplySnapshot(snapshot));
        _backend.BackendFailed += message =>
            DispatcherQueue.TryEnqueue(() => ShowError(message));
        Activated += MainWindow_Activated;
        Closed += MainWindow_Closed;
    }

    private void BuildShell()
    {
        GlobalMessage.Child = GlobalMessageText;
        RootGrid.ColumnDefinitions.Add(SidebarColumn);
        RootGrid.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        var sidebar = new StackPanel
        {
            Spacing = Tokens.CompactSpacing,
            Padding = new Thickness(14, 22, 14, 14),
        };
        sidebar.Children.Add(new TextBlock
        {
            Text = T(PresentationTextKey.AppName),
            FontSize = 20,
            FontWeight = Microsoft.UI.Text.FontWeights.SemiBold,
            Margin = new Thickness(8, 0, 8, 14),
        });
        ConfigureNavigationButton(OverviewButton, "overview");
        ConfigureNavigationButton(PackagesButton, "packages");
        ConfigureNavigationButton(LogsButton, "logs");
        ConfigureNavigationButton(UpdatesButton, "updates");
        sidebar.Children.Add(OverviewButton);
        sidebar.Children.Add(PackagesButton);
        sidebar.Children.Add(LogsButton);
        sidebar.Children.Add(UpdatesButton);
        var sidebarSurface = new Border
        {
            BorderThickness = new Thickness(0, 0, 1, 0),
            BorderBrush = new SolidColorBrush(Color.FromArgb(45, 128, 128, 128)),
            Child = sidebar,
        };
        Grid.SetColumn(sidebarSurface, 0);
        RootGrid.Children.Add(sidebarSurface);
        var content = new Grid();
        content.Children.Add(new ScrollViewer
        {
            HorizontalScrollBarVisibility = ScrollBarVisibility.Disabled,
            Content = PageHost,
        });
        content.Children.Add(GlobalMessage);
        Grid.SetColumn(content, 1);
        RootGrid.Children.Add(content);
        UpdateNavigationSelection();
    }

    private void ConfigureNavigationButton(Button button, string section)
    {
        button.HorizontalAlignment = HorizontalAlignment.Stretch;
        button.HorizontalContentAlignment = HorizontalAlignment.Left;
        button.Padding = new Thickness(12, 9, 12, 9);
        button.Click += (_, _) =>
        {
            _section = section;
            App.Log($"Navigating to {section}.");
            UpdateNavigationSelection();
            RenderCurrentPage();
        };
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
                _velopackResult = await _velopack.CheckAsync(
                    snapshot.Update.Channel,
                    snapshot.Presentation.Platform.Architecture,
                    snapshot.Presentation.Platform.RepositoryURL);
                RenderCurrentPage();
            }
        }
        catch (Exception exception)
        {
            App.Log($"Backend startup failed: {exception}");
            ShowError(exception.Message);
        }
    }

    private async void MainWindow_Closed(object sender, WindowEventArgs args)
    {
        await _backend.DisposeAsync();
    }

    private void ApplySnapshot(BackendAppSnapshot snapshot)
    {
        if (snapshot.ProtocolVersion != BackendClient.ProtocolVersion)
        {
            ShowError(T(PresentationTextKey.AppProtocolMismatch));
            return;
        }
        _snapshot = snapshot;
        ApplyChrome();
        RenderCurrentPage();
    }

    private void ApplyChrome()
    {
        Title = T(PresentationTextKey.AppName);
        RootGrid.MinWidth = Tokens.WindowMinWidth;
        RootGrid.MinHeight = Tokens.WindowMinHeight;
        SidebarColumn.Width = new GridLength(Tokens.NavigationWidth);
        OverviewButton.Content = T(PresentationTextKey.NavOverview);
        PackagesButton.Content = T(PresentationTextKey.NavPackages);
        LogsButton.Content = T(PresentationTextKey.NavLogs);
        UpdatesButton.Content = T(PresentationTextKey.NavUpdates);
        UpdateNavigationSelection();
    }

    private void RenderLoading()
    {
        SetPage(WrapPage(CreatePage(
            T(PresentationTextKey.StatusStartingTitle),
            T(PresentationTextKey.OverviewConnectingDetail))));
    }

    private void RenderCurrentPage()
    {
        if (_snapshot is null)
        {
            RenderLoading();
            return;
        }
        App.Log($"Rendering {_section} page.");
        FrameworkElement page = _section switch
        {
            "packages" => BuildPackagesPage(),
            "logs" => BuildLogsPage(),
            "updates" => BuildUpdatesPage(),
            _ => BuildOverviewPage(),
        };
        App.Log($"Built {_section} page; attaching visual tree.");
        SetPage(page);
        App.Log($"Attached {_section} page.");
    }

    private FrameworkElement BuildOverviewPage()
    {
        var snapshot = Snapshot;
        var page = CreatePage(
            T(PresentationTextKey.OverviewTitle),
            T(PresentationTextKey.OverviewSubtitle));

        var statusContent = new StackPanel { Spacing = Tokens.CompactSpacing };
        statusContent.Children.Add(new TextBlock
        {
            Text = snapshot.Presentation.Status.Title,
            FontSize = 22,
            FontWeight = Microsoft.UI.Text.FontWeights.SemiBold,
            Foreground = new SolidColorBrush(StatusToneColor(snapshot.Presentation.Status.Tone)),
        });
        var statusDetail = snapshot.Presentation.Status.Detail;
        if (!string.IsNullOrWhiteSpace(statusDetail))
        {
            statusContent.Children.Add(BodyText(statusDetail));
        }
        statusContent.Children.Add(BodyText(T(
            PresentationTextKey.PackagesEnabledSummary,
            ("enabled", snapshot.EnabledPackageCount.ToString(CultureInfo.InvariantCulture)),
            ("total", snapshot.Packages.Count.ToString(CultureInfo.InvariantCulture)),
            ("active", snapshot.ActivePackageCount.ToString(CultureInfo.InvariantCulture)))));
        page.Children.Add(Card(statusContent));

        var controls = new StackPanel { Spacing = Tokens.ControlSpacing };
        controls.Children.Add(SectionTitle(T(PresentationTextKey.OverviewControl)));
        var enabled = new ToggleSwitch
        {
            Header = T(PresentationTextKey.OverviewEnable),
            OffContent = T(PresentationTextKey.PackagesFilterDisabled),
            OnContent = T(PresentationTextKey.PackagesFilterEnabled),
            IsOn = snapshot.Enabled,
            IsEnabled = snapshot.Presentation.Actions.SetEnabled,
        };
        enabled.Toggled += async (_, _) =>
            await RunBackendAsync("setEnabled", new { enabled = enabled.IsOn });
        controls.Children.Add(enabled);
        controls.Children.Add(Caption(T(PresentationTextKey.OverviewEnableDetail)));

        var actionRow = Row();
        actionRow.Children.Add(ActionButton(
            snapshot.Presentation.Actions.OpenCodex
                ? T(PresentationTextKey.OverviewOpenCodex)
                : T(PresentationTextKey.StatusLaunchingCodexTitle),
            () => RunBackendAsync("openCodex"),
            snapshot.Presentation.Actions.OpenCodex,
            prominent: true));
        if (snapshot.Presentation.Actions.RestartCodex)
        {
            actionRow.Children.Add(ActionButton(
                T(PresentationTextKey.OverviewRestartAndConnect),
                ConfirmRestartAsync,
                true));
        }
        actionRow.Children.Add(ActionButton(
            T(PresentationTextKey.OverviewReinject),
            () => RunBackendAsync("reinject"),
            snapshot.Presentation.Actions.Reinject));
        controls.Children.Add(actionRow);

        var navigationRow = Row();
        navigationRow.Children.Add(ActionButton(
            T(PresentationTextKey.OverviewManagePackages),
            () => NavigateAsync("packages")));
        navigationRow.Children.Add(ActionButton(
            T(PresentationTextKey.OverviewViewLogs),
            () => NavigateAsync("logs")));
        controls.Children.Add(navigationRow);
        page.Children.Add(Card(controls));

        var authoring = new StackPanel { Spacing = Tokens.ControlSpacing };
        authoring.Children.Add(SectionTitle(T(PresentationTextKey.OverviewAiAuthoring)));
        authoring.Children.Add(BodyText(T(PresentationTextKey.OverviewCopySkillDetail)));
        authoring.Children.Add(ActionButton(
            T(PresentationTextKey.OverviewCopy),
            CopyAuthoringPromptAsync,
            snapshot.Presentation.Actions.ReadAuthoringPrompt));
        page.Children.Add(Card(authoring));

        var resources = new StackPanel { Spacing = Tokens.ControlSpacing };
        resources.Children.Add(SectionTitle(T(PresentationTextKey.OverviewResources)));
        resources.Children.Add(BodyText(snapshot.PackagesDirectory));
        resources.Children.Add(ActionButton(
            T(PresentationTextKey.OverviewOpenPackagesDirectory),
            () => OpenPathAsync(snapshot.PackagesDirectory),
            snapshot.Presentation.Actions.OpenPackagesDirectory));
        page.Children.Add(Card(resources));
        return WrapPage(page);
    }

    private FrameworkElement BuildPackagesPage()
    {
        var snapshot = Snapshot;
        var page = CreatePage(
            T(PresentationTextKey.PackagesTitle),
            T(PresentationTextKey.PackagesSubtitle));

        var tools = new StackPanel { Spacing = Tokens.ControlSpacing };
        tools.Children.Add(BodyText(T(
            PresentationTextKey.PackagesEnabledSummary,
            ("enabled", snapshot.EnabledPackageCount.ToString(CultureInfo.InvariantCulture)),
            ("total", snapshot.Packages.Count.ToString(CultureInfo.InvariantCulture)),
            ("active", snapshot.ActivePackageCount.ToString(CultureInfo.InvariantCulture)))));

        var developerMode = new ToggleSwitch
        {
            Header = T(PresentationTextKey.PackagesDeveloperMode),
            IsOn = snapshot.DeveloperMode,
            IsEnabled = snapshot.Presentation.Actions.SetDeveloperMode,
        };
        developerMode.Toggled += async (_, _) =>
            await RunBackendAsync("setDeveloperMode", new { enabled = developerMode.IsOn });
        tools.Children.Add(developerMode);
        tools.Children.Add(Caption(T(PresentationTextKey.PackagesDeveloperModeDetail)));

        var actionRow = Row();
        actionRow.Children.Add(ActionButton(
            T(PresentationTextKey.PackagesRescan),
            async () =>
            {
                await RunBackendAsync("reloadPackages");
                await RunBackendAsync("checkNodeEnvironment");
                await RunBackendAsync("checkGitEnvironment");
            },
            snapshot.Presentation.Actions.ReloadPackages,
            prominent: true));
        actionRow.Children.Add(LocalInstallButton());
        actionRow.Children.Add(ActionButton(
            T(PresentationTextKey.PackagesInstallRemote),
            ShowRemoteInstallDialogAsync,
            snapshot.Presentation.Actions.InstallRemotePackage));
        actionRow.Children.Add(ActionButton(
            snapshot.CheckingRemoteUpdates
                ? T(PresentationTextKey.PackagesCheckingRemote)
                : T(PresentationTextKey.PackagesCheckRemoteUpdates),
            () => RunBackendAsync("checkManagedPackageUpdates", new { automatic = false }),
            snapshot.Presentation.Actions.CheckManagedPackageUpdates));
        tools.Children.Add(actionRow);
        tools.Children.Add(EnvironmentRow(
            snapshot.CheckingNode,
            snapshot.NodeEnvironment?.Version,
            snapshot.NodeEnvironment?.NodePath,
            PresentationTextKey.PackagesNodeChecking,
            PresentationTextKey.PackagesNodeAvailable,
            PresentationTextKey.PackagesNodeMissing));
        tools.Children.Add(EnvironmentRow(
            snapshot.CheckingGit,
            snapshot.GitEnvironment?.Version,
            snapshot.GitEnvironment?.GitPath,
            PresentationTextKey.PackagesGitChecking,
            PresentationTextKey.PackagesGitAvailable,
            PresentationTextKey.PackagesGitMissing));
        AddOperationFeedback(tools, snapshot.LocalOperationMessage, snapshot.LocalOperationError);
        AddOperationFeedback(tools, snapshot.RemoteOperationMessage, snapshot.RemoteOperationError);
        page.Children.Add(Card(tools));

        var searchRow = Row();
        var search = new TextBox
        {
            PlaceholderText = T(PresentationTextKey.PackagesSearchPlaceholder),
            Text = _packageSearch,
            MinWidth = 280,
        };
        search.KeyDown += (_, args) =>
        {
            if (args.Key == VirtualKey.Enter)
            {
                _packageSearch = search.Text.Trim();
                RenderCurrentPage();
            }
        };
        searchRow.Children.Add(search);
        searchRow.Children.Add(ActionButton(
            T(PresentationTextKey.PackagesFilter),
            () =>
            {
                _packageSearch = search.Text.Trim();
                RenderCurrentPage();
                return Task.CompletedTask;
            }));
        if (!string.IsNullOrEmpty(_packageSearch))
        {
            searchRow.Children.Add(ActionButton(
                T(PresentationTextKey.PackagesClearSearch),
                () =>
                {
                    _packageSearch = string.Empty;
                    RenderCurrentPage();
                    return Task.CompletedTask;
                }));
        }
        page.Children.Add(searchRow);

        var packages = snapshot.Packages.Where(package =>
            string.IsNullOrEmpty(_packageSearch)
            || package.DisplayName.Contains(_packageSearch, StringComparison.CurrentCultureIgnoreCase)
            || package.Id.Contains(_packageSearch, StringComparison.OrdinalIgnoreCase)).ToList();
        if (packages.Count == 0)
        {
            page.Children.Add(Card(new StackPanel
            {
                Spacing = Tokens.CompactSpacing,
                Children =
                {
                    SectionTitle(T(string.IsNullOrEmpty(_packageSearch)
                        ? PresentationTextKey.PackagesEmptyTitle
                        : PresentationTextKey.PackagesNoMatchTitle)),
                    Caption(T(string.IsNullOrEmpty(_packageSearch)
                        ? PresentationTextKey.PackagesEmptyDetail
                        : PresentationTextKey.PackagesNoMatchDetail)),
                },
            }));
        }
        else
        {
            foreach (var package in packages)
            {
                page.Children.Add(BuildPackageCard(package));
            }
        }
        return WrapPage(page);
    }

    private FrameworkElement BuildPackageCard(PackageView package)
    {
        var snapshot = Snapshot;
        var content = new StackPanel { Spacing = Tokens.CompactSpacing };
        var header = Row();
        var toggle = new ToggleSwitch
        {
            IsOn = !snapshot.DisabledPackageIds.Contains(package.Id),
            IsEnabled = package.AvailableActions.SetEnabled,
            MinWidth = 44,
        };
        toggle.Toggled += async (_, _) =>
            await RunBackendAsync("setPackageEnabled", new { packageID = package.Id, enabled = toggle.IsOn });
        header.Children.Add(toggle);
        header.Children.Add(new TextBlock
        {
            Text = package.DisplayName,
            FontSize = 18,
            FontWeight = Microsoft.UI.Text.FontWeights.SemiBold,
            VerticalAlignment = VerticalAlignment.Center,
        });
        header.Children.Add(Caption(T(
            PresentationTextKey.PackagesSourceVersion,
            ("version", package.DisplayVersion))));
        header.Children.Add(new TextBlock
        {
            Text = package.Presentation.StatusTitle,
            Foreground = new SolidColorBrush(StatusToneColor(package.Presentation.StatusTone)),
            VerticalAlignment = VerticalAlignment.Center,
        });
        content.Children.Add(header);
        content.Children.Add(BodyText(string.IsNullOrWhiteSpace(package.Detail)
            ? T(PresentationTextKey.PackagesNoDescription)
            : package.Detail));
        if (!string.IsNullOrWhiteSpace(package.Presentation.StatusDetail))
        {
            content.Children.Add(new TextBlock
            {
                Text = package.Presentation.StatusDetail,
                TextWrapping = TextWrapping.Wrap,
                Foreground = new SolidColorBrush(StatusToneColor(package.Presentation.StatusTone)),
                IsTextSelectionEnabled = true,
            });
        }

        foreach (var message in PackageMessages(package))
        {
            content.Children.Add(new TextBlock
            {
                Text = message,
                TextWrapping = TextWrapping.Wrap,
                Foreground = new SolidColorBrush(ParseColor(Tokens.WarningColor)),
            });
        }

        var actions = Row();
        var priority = new TextBox
        {
            Header = T(PresentationTextKey.PackagesPriority),
            Text = package.Priority.ToString(CultureInfo.InvariantCulture),
            Width = 120,
            IsEnabled = package.AvailableActions.SetPriority,
        };
        var applyingPriority = false;
        async Task ApplyPriorityAsync()
        {
            if (applyingPriority)
            {
                return;
            }

            if (!int.TryParse(
                    priority.Text,
                    NumberStyles.Integer,
                    CultureInfo.InvariantCulture,
                    out var value))
            {
                priority.Text = package.Priority.ToString(CultureInfo.InvariantCulture);
                return;
            }
            if (value == package.Priority)
            {
                return;
            }

            applyingPriority = true;
            try
            {
                await RunBackendAsync(
                    "setPackagePriority",
                    new { packageID = package.Id, priority = value });
            }
            finally
            {
                applyingPriority = false;
            }
        }
        priority.KeyDown += async (_, eventArgs) =>
        {
            if (eventArgs.Key == VirtualKey.Enter)
            {
                await ApplyPriorityAsync();
            }
        };
        priority.LostFocus += async (_, _) => await ApplyPriorityAsync();
        actions.Children.Add(priority);
        if (package.PriorityOverride is not null)
        {
            actions.Children.Add(ActionButton(
                T(PresentationTextKey.PackagesResetPriority, ("priority", package.DeclaredPriority.ToString())),
                () => RunBackendAsync("setPackagePriority", new { packageID = package.Id, priority = (int?)null }),
                package.AvailableActions.SetPriority));
        }
        if (package.CanInstallMissingDependencies)
        {
            actions.Children.Add(ActionButton(
                T(PresentationTextKey.PackagesInstallDependencies),
                () => RunBackendAsync("installMissingDependencies", new { packageID = package.Id }),
                package.AvailableActions.InstallMissingDependencies));
        }
        if (package.CanEnableDependencies)
        {
            actions.Children.Add(ActionButton(
                T(PresentationTextKey.PackagesEnableDependencies),
                () => RunBackendAsync("enableDependencies", new { packageID = package.Id }),
                package.AvailableActions.EnableDependencies));
        }
        if (snapshot.RemotePackageUpdates.TryGetValue(package.Id, out var remote)
            && remote.Status == "available")
        {
            actions.Children.Add(ActionButton(
                T(PresentationTextKey.PackagesUpdateAndBuild),
                () => RunBackendAsync("updateManagedPackage", new { packageID = package.Id }),
                package.AvailableActions.UpdateManagedPackage,
                prominent: true));
        }
        actions.Children.Add(ActionButton(
            T(PresentationTextKey.PackagesOpenDirectory),
            () => OpenPathAsync(package.Directory),
            package.AvailableActions.OpenDirectory));
        actions.Children.Add(ActionButton(
            snapshot.BuildingPackageIds.Contains(package.Id)
                ? T(PresentationTextKey.PackagesBuilding)
                : T(package.BuildDisposition == "current"
                    ? PresentationTextKey.PackagesRebuild
                    : PresentationTextKey.PackagesBuild),
            () => RunBackendAsync("buildPackage", new { packageID = package.Id }),
            package.AvailableActions.Build,
            prominent: true));
        content.Children.Add(actions);
        return Card(content);
    }

    private FrameworkElement BuildLogsPage()
    {
        var snapshot = Snapshot;
        var page = CreatePage(
            T(PresentationTextKey.LogsTitle),
            T(PresentationTextKey.LogsSubtitle));
        var actions = Row();
        actions.Children.Add(ActionButton(
            T(PresentationTextKey.LogsRefresh),
            () => RunBackendAsync("refreshLog"),
            snapshot.Presentation.Actions.RefreshLog,
            prominent: true));
        actions.Children.Add(ActionButton(
            T(PresentationTextKey.LogsOpenFile),
            () => OpenPathAsync(snapshot.LogPath),
            snapshot.Presentation.Actions.OpenLogFile));
        actions.Children.Add(ActionButton(
            T(PresentationTextKey.LogsClear),
            ConfirmClearLogAsync,
            snapshot.Presentation.Actions.ClearLog));
        page.Children.Add(actions);
        page.Children.Add(Card(new TextBox
        {
            Text = snapshot.LogText,
            IsReadOnly = true,
            AcceptsReturn = true,
            TextWrapping = TextWrapping.NoWrap,
            FontFamily = new FontFamily("Consolas"),
            MinHeight = 500,
        }));
        return WrapPage(page);
    }

    private FrameworkElement BuildUpdatesPage()
    {
        var snapshot = Snapshot;
        var page = CreatePage(
            T(PresentationTextKey.UpdateTitle),
            T(PresentationTextKey.UpdateSubtitle));

        var application = new StackPanel { Spacing = Tokens.CompactSpacing };
        application.Children.Add(SectionTitle(T(PresentationTextKey.AppName)));
        application.Children.Add(BodyText(T(
            PresentationTextKey.UpdateVersionBuild,
            ("version", snapshot.Update.CurrentVersion),
            ("build", snapshot.Update.BuildNumber))));
        application.Children.Add(ActionButton(
            T(PresentationTextKey.UpdateRepository),
            () => OpenPathAsync(snapshot.Presentation.Platform.RepositoryURL),
            snapshot.Presentation.Actions.OpenRepository));
        page.Children.Add(Card(application));

        var updates = new StackPanel { Spacing = Tokens.ControlSpacing };
        updates.Children.Add(SectionTitle(T(PresentationTextKey.UpdateSoftwareUpdate)));
        var channel = new ComboBox
        {
            Header = T(PresentationTextKey.UpdateChannel),
            MinWidth = 240,
            IsEnabled = snapshot.Presentation.Actions.SetUpdatePreferences,
        };
        channel.Items.Add(new ComboBoxItem
        {
            Content = T(PresentationTextKey.UpdateChannelStable),
            Tag = "stable",
        });
        channel.Items.Add(new ComboBoxItem
        {
            Content = T(PresentationTextKey.UpdateChannelBeta),
            Tag = "beta",
        });
        channel.SelectedIndex = snapshot.Update.Channel == "beta" ? 1 : 0;
        channel.SelectionChanged += async (_, _) =>
        {
            if (channel.SelectedItem is ComboBoxItem selected
                && selected.Tag is string selectedChannel
                && selectedChannel != Snapshot.Update.Channel)
            {
                _velopackResult = null;
                await RunBackendAsync("setUpdateChannel", new { channel = selectedChannel });
            }
        };
        updates.Children.Add(channel);
        updates.Children.Add(Caption(T(snapshot.Update.Channel == "beta"
            ? PresentationTextKey.UpdateChannelBetaDetail
            : PresentationTextKey.UpdateChannelStableDetail)));

        var autoCheck = new ToggleSwitch
        {
            Header = T(PresentationTextKey.UpdateAutoCheck),
            IsOn = snapshot.Update.AutoCheck,
            IsEnabled = snapshot.Presentation.Actions.SetUpdatePreferences,
        };
        autoCheck.Toggled += async (_, _) =>
        {
            if (autoCheck.IsOn != Snapshot.Update.AutoCheck)
            {
                await RunBackendAsync("setUpdateAutoCheck", new { enabled = autoCheck.IsOn });
            }
        };
        updates.Children.Add(autoCheck);
        updates.Children.Add(BodyText($"{T(PresentationTextKey.UpdateCurrentVersion)}: {snapshot.Update.CurrentVersion}"));
        updates.Children.Add(BodyText($"{T(PresentationTextKey.UpdateLatestVersion)}: {snapshot.Update.LatestVersionString}"));
        updates.Children.Add(BodyText($"{T(PresentationTextKey.UpdateLastCheck)}: {FormatLastCheck(snapshot.Update.LastCheckAt)}"));
        if (!string.IsNullOrWhiteSpace(snapshot.Update.LastError))
        {
            updates.Children.Add(ErrorText(snapshot.Update.LastError));
        }
        if (_velopackResult?.Installed == false)
        {
            updates.Children.Add(Caption(T(PresentationTextKey.UpdateNotInstalled)));
        }
        if (!string.IsNullOrWhiteSpace(_velopackResult?.Error))
        {
            updates.Children.Add(ErrorText(T(
                PresentationTextKey.UpdateCheckFailed,
                ("message", _velopackResult.Error!))));
        }

        var actionRow = Row();
        actionRow.Children.Add(ActionButton(
            snapshot.Update.Checking || _applyingUpdate
                ? T(PresentationTextKey.UpdateChecking)
                : T(PresentationTextKey.UpdateCheck),
            CheckUpdatesAsync,
            snapshot.Presentation.Actions.CheckAppUpdate && !_applyingUpdate,
            prominent: true));
        if (_velopackResult is { Installed: true, Version: not null })
        {
            actionRow.Children.Add(ActionButton(
                _applyingUpdate
                    ? T(PresentationTextKey.UpdateApplyProgress)
                    : T(PresentationTextKey.UpdateInstall, ("version", _velopackResult.Version)),
                InstallUpdateAsync,
                snapshot.Presentation.Actions.InstallAppUpdate && !_applyingUpdate,
                prominent: true));
        }
        else if (snapshot.Update.LatestRelease?.HtmlUrl is { } releaseUrl)
        {
            actionRow.Children.Add(ActionButton(
                T(PresentationTextKey.UpdateViewRelease),
                () => OpenPathAsync(releaseUrl)));
        }
        updates.Children.Add(actionRow);
        page.Children.Add(Card(updates));
        return WrapPage(page);
    }

    private Button LocalInstallButton()
    {
        var button = new Button
        {
            Content = T(PresentationTextKey.PackagesInstallLocal),
            IsEnabled = Snapshot.Presentation.Actions.InstallLocalPackage,
        };
        var menu = new MenuFlyout();
        var zip = new MenuFlyoutItem { Text = T(PresentationTextKey.PackagesChooseZip) };
        zip.Click += async (_, _) => await PickLocalZipAsync();
        menu.Items.Add(zip);
        var directory = new MenuFlyoutItem { Text = T(PresentationTextKey.PackagesChooseDirectory) };
        directory.Click += async (_, _) => await PickLocalDirectoryAsync();
        menu.Items.Add(directory);
        button.Flyout = menu;
        return button;
    }

    private async Task PickLocalZipAsync()
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

    private async Task PickLocalDirectoryAsync()
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

    private async Task ShowRemoteInstallDialogAsync()
    {
        var repository = new TextBox
        {
            Header = T(PresentationTextKey.RemoteRepository),
            PlaceholderText = T(PresentationTextKey.RemoteRepositoryPlaceholder),
        };
        var selector = new ComboBox
        {
            Header = T(PresentationTextKey.RemoteSelector),
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
            selector.Items.Add(new ComboBoxItem { Content = T(option.Key), Tag = option.Value });
        }
        selector.SelectedIndex = 0;
        var selectorValue = new TextBox
        {
            Header = T(PresentationTextKey.SelectorBranchValue),
            Text = T(PresentationTextKey.SelectorBranchDefault),
        };
        selector.SelectionChanged += (_, _) =>
        {
            var selected = selectorOptions[selector.SelectedIndex];
            selectorValue.IsEnabled = selected.RequiresValue;
            selectorValue.Header = selected.Value switch
            {
                "tag" => T(PresentationTextKey.SelectorTagValue),
                "githubRelease" => T(PresentationTextKey.SelectorGithubReleaseValue),
                "commit" => T(PresentationTextKey.SelectorCommitValue),
                _ => T(PresentationTextKey.SelectorBranchValue),
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
        content.Children.Add(Caption(T(PresentationTextKey.RemoteValidationDetail)));
        var dialog = new ContentDialog
        {
            XamlRoot = PageHost.XamlRoot,
            Title = T(PresentationTextKey.RemoteTitle),
            Content = content,
            PrimaryButtonText = T(PresentationTextKey.RemoteInstall),
            CloseButtonText = T(PresentationTextKey.RemoteClose),
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

    private async Task ConfirmRestartAsync()
    {
        var dialog = new ContentDialog
        {
            XamlRoot = PageHost.XamlRoot,
            Title = T(PresentationTextKey.DialogRestartTitle),
            Content = T(PresentationTextKey.DialogRestartMessage),
            PrimaryButtonText = T(PresentationTextKey.OverviewRestartAndConnect),
            CloseButtonText = T(PresentationTextKey.CommonCancel),
            DefaultButton = ContentDialogButton.Close,
        };
        if (await dialog.ShowAsync() == ContentDialogResult.Primary)
        {
            await RunBackendAsync("restartCodex");
        }
    }

    private async Task ConfirmClearLogAsync()
    {
        var dialog = new ContentDialog
        {
            XamlRoot = PageHost.XamlRoot,
            Title = T(PresentationTextKey.LogsClearTitle),
            Content = T(PresentationTextKey.LogsClearMessage),
            PrimaryButtonText = T(PresentationTextKey.LogsClear),
            CloseButtonText = T(PresentationTextKey.CommonCancel),
            DefaultButton = ContentDialogButton.Close,
        };
        if (await dialog.ShowAsync() == ContentDialogResult.Primary)
        {
            await RunBackendAsync("clearLog");
        }
    }

    private async Task CopyAuthoringPromptAsync()
    {
        try
        {
            var prompt = await _backend.ReadAuthoringPromptAsync();
            var data = new DataPackage();
            data.SetText(prompt);
            Clipboard.SetContent(data);
            ShowMessage(T(PresentationTextKey.OverviewCopied), isError: false);
        }
        catch (Exception exception)
        {
            ShowError(exception.Message);
        }
    }

    private async Task CheckUpdatesAsync()
    {
        try
        {
            await _backend.SendAsync("checkAppUpdate", new { prompt = false });
            _velopackResult = await _velopack.CheckAsync(
                Snapshot.Update.Channel,
                Snapshot.Presentation.Platform.Architecture,
                Snapshot.Presentation.Platform.RepositoryURL);
            RenderCurrentPage();
        }
        catch (Exception exception)
        {
            ShowError(T(PresentationTextKey.UpdateCheckFailed, ("message", exception.Message)));
        }
    }

    private async Task InstallUpdateAsync()
    {
        _applyingUpdate = true;
        RenderCurrentPage();
        try
        {
            await _velopack.DownloadAndApplyAsync();
        }
        catch (Exception exception)
        {
            _applyingUpdate = false;
            ShowError(T(PresentationTextKey.UpdateInstallFailed, ("message", exception.Message)));
            RenderCurrentPage();
        }
    }

    private async Task RunBackendAsync(string method, object? parameters = null)
    {
        try
        {
            await _backend.SendAsync(method, parameters);
        }
        catch (Exception exception)
        {
            ShowError(exception.Message);
        }
    }

    private Task NavigateAsync(string section)
    {
        _section = section;
        UpdateNavigationSelection();
        RenderCurrentPage();
        return Task.CompletedTask;
    }

    private void UpdateNavigationSelection()
    {
        foreach (var (button, section) in new[]
        {
            (OverviewButton, "overview"),
            (PackagesButton, "packages"),
            (LogsButton, "logs"),
            (UpdatesButton, "updates"),
        })
        {
            var selected = _section == section;
            button.FontWeight = selected
                ? Microsoft.UI.Text.FontWeights.SemiBold
                : Microsoft.UI.Text.FontWeights.Normal;
            button.Opacity = selected ? 1 : 0.72;
        }
    }

    private static Task OpenPathAsync(string path)
    {
        if (!string.IsNullOrWhiteSpace(path))
        {
            Process.Start(new ProcessStartInfo(path) { UseShellExecute = true });
        }
        return Task.CompletedTask;
    }

    private StackPanel EnvironmentRow(
        bool checking,
        string? version,
        string? path,
        string checkingKey,
        string availableKey,
        string missingKey)
    {
        var row = new StackPanel { Spacing = 2 };
        if (checking)
        {
            row.Children.Add(BodyText(T(checkingKey)));
        }
        else if (!string.IsNullOrWhiteSpace(version))
        {
            row.Children.Add(BodyText(T(availableKey, ("version", version))));
            row.Children.Add(Caption(path ?? string.Empty));
        }
        else
        {
            row.Children.Add(Caption(T(missingKey)));
        }
        return row;
    }

    private void AddOperationFeedback(StackPanel parent, string? message, string? error)
    {
        if (!string.IsNullOrWhiteSpace(message))
        {
            parent.Children.Add(new TextBlock
            {
                Text = message,
                TextWrapping = TextWrapping.Wrap,
                Foreground = new SolidColorBrush(ParseColor(Tokens.SuccessColor)),
            });
        }
        if (!string.IsNullOrWhiteSpace(error))
        {
            parent.Children.Add(ErrorText(error));
        }
    }

    private IEnumerable<string> PackageMessages(PackageView package)
    {
        if (!string.IsNullOrWhiteSpace(package.ValidationError))
        {
            yield return package.ValidationError;
        }
        if (Snapshot.PackageBuildErrors.TryGetValue(package.Id, out var buildError))
        {
            yield return buildError;
        }
        if (Snapshot.PackageRuntimeErrors.TryGetValue(package.Id, out var runtimeError))
        {
            yield return runtimeError;
        }
        if (Snapshot.PackagePayloadErrors.TryGetValue(package.Id, out var payloadError))
        {
            yield return payloadError;
        }
        if (Snapshot.RemotePackageErrors.TryGetValue(package.Id, out var remoteError))
        {
            yield return remoteError;
        }
        if (Snapshot.PackageDependencyIssues.TryGetValue(package.Id, out var dependencyIssues))
        {
            foreach (var issue in dependencyIssues)
            {
                yield return issue;
            }
        }
    }

    private Color StatusToneColor(string tone)
    {
        return tone switch
        {
            "success" => ParseColor(Tokens.SuccessColor),
            "danger" => ParseColor(Tokens.DangerColor),
            "warning" => ParseColor(Tokens.WarningColor),
            "neutral" => Colors.Gray,
            _ => ParseColor(Tokens.AccentColor),
        };
    }

    private StackPanel CreatePage(string title, string subtitle)
    {
        var page = new StackPanel
        {
            Spacing = Tokens.SectionSpacing,
            MaxWidth = Tokens.ContentMaxWidth,
            HorizontalAlignment = HorizontalAlignment.Stretch,
        };
        page.Children.Add(new StackPanel
        {
            Spacing = Tokens.CompactSpacing,
            Children =
            {
                new TextBlock
                {
                    Text = title,
                    FontSize = 32,
                    FontWeight = Microsoft.UI.Text.FontWeights.SemiBold,
                    TextWrapping = TextWrapping.Wrap,
                },
                Caption(subtitle),
            },
        });
        return page;
    }

    private FrameworkElement WrapPage(StackPanel page)
    {
        var wrapper = new Grid
        {
            Padding = new Thickness(Tokens.PagePadding),
            HorizontalAlignment = HorizontalAlignment.Stretch,
        };
        wrapper.Children.Add(page);
        return wrapper;
    }

    private void SetPage(FrameworkElement page)
    {
        PageHost.Children.Clear();
        PageHost.Children.Add(page);
    }

    private Border Card(UIElement content)
    {
        return new Border
        {
            Padding = new Thickness(Tokens.CardPadding),
            CornerRadius = new CornerRadius(Tokens.CardCornerRadius),
            BorderThickness = new Thickness(1),
            BorderBrush = new SolidColorBrush(Color.FromArgb(45, 128, 128, 128)),
            Background = new SolidColorBrush(Color.FromArgb(12, 128, 128, 128)),
            Child = content,
        };
    }

    private StackPanel Row()
    {
        return new StackPanel
        {
            Orientation = Orientation.Horizontal,
            Spacing = Tokens.ControlSpacing,
        };
    }

    private static TextBlock SectionTitle(string value)
    {
        return new TextBlock
        {
            Text = value,
            FontSize = 20,
            FontWeight = Microsoft.UI.Text.FontWeights.SemiBold,
            TextWrapping = TextWrapping.Wrap,
        };
    }

    private static TextBlock BodyText(string value)
    {
        return new TextBlock
        {
            Text = value,
            TextWrapping = TextWrapping.Wrap,
            IsTextSelectionEnabled = true,
        };
    }

    private static TextBlock Caption(string value)
    {
        return new TextBlock
        {
            Text = value,
            TextWrapping = TextWrapping.Wrap,
            Opacity = 0.72,
            IsTextSelectionEnabled = true,
        };
    }

    private TextBlock ErrorText(string value)
    {
        return new TextBlock
        {
            Text = value,
            TextWrapping = TextWrapping.Wrap,
            Foreground = new SolidColorBrush(ParseColor(Tokens.DangerColor)),
            IsTextSelectionEnabled = true,
        };
    }

    private Button ActionButton(
        string title,
        Func<Task> action,
        bool enabled = true,
        bool prominent = false)
    {
        var button = new Button
        {
            Content = title,
            IsEnabled = enabled,
        };
        if (prominent)
        {
            button.Style = Application.Current.Resources["AccentButtonStyle"] as Style;
        }
        button.Click += async (_, _) =>
        {
            button.IsEnabled = false;
            await action();
            if (button.XamlRoot is not null)
            {
                button.IsEnabled = enabled;
            }
        };
        return button;
    }

    private string FormatLastCheck(DateTimeOffset? value)
    {
        return value?.ToLocalTime().ToString("g", CultureInfo.CurrentCulture)
            ?? T(PresentationTextKey.UpdateNever);
    }

    private string T(string key, params (string Name, string Value)[] replacements)
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

    private PresentationTokens Tokens => _snapshot?.Presentation.Tokens ?? PresentationDefaults.Tokens;

    private BackendAppSnapshot Snapshot => _snapshot
        ?? throw new InvalidOperationException(
            PresentationFallback.Text(PresentationTextKey.AppBackendNotRunning));

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

    private void ShowError(string message)
    {
        ShowMessage(message, isError: true);
    }

    private void ShowMessage(string message, bool isError)
    {
        GlobalMessageText.Text = message;
        GlobalMessageText.Foreground = new SolidColorBrush(ParseColor(
            isError ? Tokens.DangerColor : Tokens.SuccessColor));
        GlobalMessage.Background = new SolidColorBrush(Color.FromArgb(28, 128, 128, 128));
        GlobalMessage.Visibility = Visibility.Visible;
    }
}
