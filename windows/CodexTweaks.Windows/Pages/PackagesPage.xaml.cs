using System.Globalization;
using CodexTweaks.Windows.Generated;
using CodexTweaks.Windows.Models;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Xaml.Media;
using Windows.System;

namespace CodexTweaks.Windows.Pages;

public sealed partial class PackagesPage : Page
{
    private enum PackageFilter
    {
        All,
        Enabled,
        Disabled,
        Pending,
        Error,
    }

    private sealed record PackageFilterOption(PackageFilter Value, string Title)
    {
        public override string ToString() => Title;
    }

    private MainWindow? _host;
    private BackendAppSnapshot? _snapshot;
    private List<PackageRowViewModel> _allRows = [];
    private PackageFilter _filter = PackageFilter.All;
    private string _search = string.Empty;
    private bool _rendering;

    public PackagesPage()
    {
        InitializeComponent();
    }

    internal void Render(MainWindow host, BackendAppSnapshot snapshot)
    {
        _host = host;
        _snapshot = snapshot;
        _rendering = true;
        try
        {
            PageTitle.Text = host.Text(PresentationTextKey.PackagesTitle);
            PageSubtitle.Text = host.Text(PresentationTextKey.PackagesSubtitle);
            PackageSummary.Text = host.Text(
                PresentationTextKey.PackagesEnabledSummary,
                ("enabled", snapshot.EnabledPackageCount.ToString(CultureInfo.InvariantCulture)),
                ("total", snapshot.Packages.Count.ToString(CultureInfo.InvariantCulture)),
                ("active", snapshot.ActivePackageCount.ToString(CultureInfo.InvariantCulture)));

            DeveloperModeTitle.Text = host.Text(PresentationTextKey.PackagesDeveloperMode);
            DeveloperModeDetail.Text = host.Text(PresentationTextKey.PackagesDeveloperModeDetail);
            DeveloperModeDetail.Visibility = snapshot.DeveloperMode ? Visibility.Visible : Visibility.Collapsed;
            DeveloperModeToggle.IsOn = snapshot.DeveloperMode;
            DeveloperModeToggle.IsEnabled = snapshot.Presentation.Actions.SetDeveloperMode;

            RescanButtonText.Text = host.Text(PresentationTextKey.PackagesRescan);
            RescanButton.IsEnabled = snapshot.Presentation.Actions.ReloadPackages;
            LocalInstallButtonText.Text = host.Text(snapshot.InstallingLocalPackage
                ? PresentationTextKey.PackagesInstalling
                : PresentationTextKey.PackagesInstallLocal);
            LocalInstallButton.IsEnabled = snapshot.Presentation.Actions.InstallLocalPackage;
            ChooseZipMenuItem.Text = host.Text(PresentationTextKey.PackagesChooseZip);
            ChooseDirectoryMenuItem.Text = host.Text(PresentationTextKey.PackagesChooseDirectory);
            RemoteInstallButtonText.Text = host.Text(PresentationTextKey.PackagesInstallRemote);
            RemoteInstallButton.IsEnabled = snapshot.Presentation.Actions.InstallRemotePackage;
            RemoteUpdatesButtonText.Text = host.Text(snapshot.CheckingRemoteUpdates
                ? PresentationTextKey.PackagesCheckingRemote
                : PresentationTextKey.PackagesCheckRemoteUpdates);
            RemoteUpdatesButton.IsEnabled = snapshot.Presentation.Actions.CheckManagedPackageUpdates;

            RenderEnvironment(
                NodeStatusIcon,
                NodeStatusText,
                NodePathText,
                snapshot.CheckingNode,
                snapshot.NodeEnvironment?.Version,
                snapshot.NodeEnvironment?.NodePath,
                PresentationTextKey.PackagesNodeChecking,
                PresentationTextKey.PackagesNodeAvailable,
                PresentationTextKey.PackagesNodeMissing);
            RenderEnvironment(
                GitStatusIcon,
                GitStatusText,
                GitPathText,
                snapshot.CheckingGit,
                snapshot.GitEnvironment?.Version,
                snapshot.GitEnvironment?.GitPath,
                PresentationTextKey.PackagesGitChecking,
                PresentationTextKey.PackagesGitAvailable,
                PresentationTextKey.PackagesGitMissing);
            RenderFeedback(LocalFeedback, snapshot.LocalOperationMessage, snapshot.LocalOperationError);
            RenderFeedback(RemoteFeedback, snapshot.RemoteOperationMessage, snapshot.RemoteOperationError);

            LoadOrderTitle.Text = host.Text(PresentationTextKey.PackagesLoadOrder);
            LoadOrderDetail.Text = host.Text(PresentationTextKey.PackagesLoadOrderDetail);
            SearchBox.PlaceholderText = host.Text(PresentationTextKey.PackagesSearchPlaceholder);
            if (SearchBox.Text != _search)
            {
                SearchBox.Text = _search;
            }
            FilterComboBox.ItemsSource = FilterOptions(host);
            FilterComboBox.SelectedItem = ((IEnumerable<PackageFilterOption>)FilterComboBox.ItemsSource)
                .First(option => option.Value == _filter);
            AutomationProperties.SetName(FilterComboBox, host.Text(PresentationTextKey.PackagesFilter));
            AutomationProperties.SetName(ClearFilterButton, host.Text(PresentationTextKey.PackagesClearSearch));
            ToolTipService.SetToolTip(ClearFilterButton, host.Text(PresentationTextKey.PackagesClearSearch));

            _allRows = snapshot.Packages
                .Select(package => new PackageRowViewModel(host, snapshot, package))
                .ToList();
            ApplyFilters();
        }
        finally
        {
            _rendering = false;
        }
    }

    private IReadOnlyList<PackageFilterOption> FilterOptions(MainWindow host) =>
    [
        new(PackageFilter.All, host.Text(PresentationTextKey.PackagesFilterAll)),
        new(PackageFilter.Enabled, host.Text(PresentationTextKey.PackagesFilterEnabled)),
        new(PackageFilter.Disabled, host.Text(PresentationTextKey.PackagesFilterDisabled)),
        new(PackageFilter.Pending, host.Text(PresentationTextKey.PackagesFilterPending)),
        new(PackageFilter.Error, host.Text(PresentationTextKey.PackagesFilterError)),
    ];

    private void ApplyFilters()
    {
        if (_host is null || _snapshot is null)
        {
            return;
        }

        var query = _search.Trim();
        var filtered = _allRows.Where(row =>
        {
            var matchesFilter = _filter switch
            {
                PackageFilter.Enabled => row.IsEnabled,
                PackageFilter.Disabled => !row.IsEnabled,
                PackageFilter.Pending => row.Package.Presentation.IsPending,
                PackageFilter.Error => row.Package.Presentation.IsError,
                _ => true,
            };
            if (!matchesFilter || query.Length == 0)
            {
                return matchesFilter;
            }
            return new[]
            {
                row.DisplayName,
                row.Id,
                row.Detail,
                row.Package.DisplayVersion,
                row.Package.Directory,
                row.StatusTitle,
                row.StatusDetail,
                row.MessagesText,
            }.Any(value => value.Contains(query, StringComparison.CurrentCultureIgnoreCase));
        }).ToList();

        PackagesList.ItemsSource = filtered;
        PackagesList.Visibility = filtered.Count == 0 ? Visibility.Collapsed : Visibility.Visible;
        EmptyState.Visibility = filtered.Count == 0 ? Visibility.Visible : Visibility.Collapsed;
        ClearFilterButton.Visibility = IsFiltering ? Visibility.Visible : Visibility.Collapsed;
        if (filtered.Count == 0)
        {
            var hasPackages = _snapshot.Packages.Count > 0;
            EmptyStateIcon.Symbol = hasPackages ? Symbol.Filter : Symbol.Library;
            EmptyStateTitle.Text = _host.Text(hasPackages
                ? PresentationTextKey.PackagesNoMatchTitle
                : PresentationTextKey.PackagesEmptyTitle);
            EmptyStateDetail.Text = _host.Text(hasPackages
                ? PresentationTextKey.PackagesNoMatchDetail
                : PresentationTextKey.PackagesEmptyDetail);
            EmptyStateButton.Content = _host.Text(hasPackages
                ? PresentationTextKey.PackagesClearSearch
                : PresentationTextKey.OverviewOpenPackagesDirectory);
        }
    }

    private bool IsFiltering => _filter != PackageFilter.All || !string.IsNullOrWhiteSpace(_search);

    private void RenderEnvironment(
        SymbolIcon icon,
        TextBlock title,
        TextBlock pathText,
        bool checking,
        string? version,
        string? path,
        string checkingKey,
        string availableKey,
        string missingKey)
    {
        if (checking)
        {
            icon.Symbol = Symbol.Sync;
            icon.Foreground = Host.ToneBrush("accent");
            title.Text = Host.Text(checkingKey);
            pathText.Visibility = Visibility.Collapsed;
        }
        else if (!string.IsNullOrWhiteSpace(version))
        {
            icon.Symbol = Symbol.Accept;
            icon.Foreground = Host.ToneBrush("success");
            title.Text = Host.Text(availableKey, ("version", version));
            pathText.Text = path ?? string.Empty;
            pathText.Visibility = Visibility.Visible;
        }
        else
        {
            icon.Symbol = Symbol.Important;
            icon.Foreground = Host.ToneBrush("warning");
            title.Text = Host.Text(missingKey);
            pathText.Visibility = Visibility.Collapsed;
        }
    }

    private static void RenderFeedback(InfoBar infoBar, string? message, string? error)
    {
        if (!string.IsNullOrWhiteSpace(error))
        {
            infoBar.Severity = InfoBarSeverity.Error;
            infoBar.Title = error;
            infoBar.Message = string.Empty;
            infoBar.IsOpen = true;
        }
        else if (!string.IsNullOrWhiteSpace(message))
        {
            infoBar.Severity = InfoBarSeverity.Success;
            infoBar.Title = message;
            infoBar.Message = string.Empty;
            infoBar.IsOpen = true;
        }
        else
        {
            infoBar.IsOpen = false;
        }
    }

    private async void DeveloperModeToggle_Toggled(object sender, RoutedEventArgs e)
    {
        if (_rendering
            || _host is null
            || _snapshot is null
            || DeveloperModeToggle.IsOn == _snapshot.DeveloperMode)
        {
            return;
        }
        await _host.RunBackendAsync("setDeveloperMode", new { enabled = DeveloperModeToggle.IsOn });
    }

    private async void RescanButton_Click(object sender, RoutedEventArgs e)
    {
        RescanButton.IsEnabled = false;
        try
        {
            await Host.RunBackendAsync("reloadPackages");
            await Host.RunBackendAsync("checkNodeEnvironment");
            await Host.RunBackendAsync("checkGitEnvironment");
        }
        finally
        {
            if (RescanButton.XamlRoot is not null)
            {
                RescanButton.IsEnabled = true;
            }
        }
    }

    private async void ChooseZipMenuItem_Click(object sender, RoutedEventArgs e) =>
        await Host.PickLocalZipAsync();

    private async void ChooseDirectoryMenuItem_Click(object sender, RoutedEventArgs e) =>
        await Host.PickLocalDirectoryAsync();

    private async void RemoteInstallButton_Click(object sender, RoutedEventArgs e) =>
        await Host.ShowRemoteInstallDialogAsync();

    private async void RemoteUpdatesButton_Click(object sender, RoutedEventArgs e) =>
        await Host.RunBackendAsync("checkManagedPackageUpdates", new { automatic = false });

    private void SearchBox_TextChanged(AutoSuggestBox sender, AutoSuggestBoxTextChangedEventArgs args)
    {
        if (_rendering || args.Reason != AutoSuggestionBoxTextChangeReason.UserInput)
        {
            return;
        }
        _search = sender.Text;
        ApplyFilters();
    }

    private void FilterComboBox_SelectionChanged(object sender, SelectionChangedEventArgs e)
    {
        if (_rendering || FilterComboBox.SelectedItem is not PackageFilterOption option)
        {
            return;
        }
        _filter = option.Value;
        ApplyFilters();
    }

    private void ClearFilterButton_Click(object sender, RoutedEventArgs e) => ClearFilters();

    private async void EmptyStateButton_Click(object sender, RoutedEventArgs e)
    {
        if (_snapshot?.Packages.Count > 0)
        {
            ClearFilters();
        }
        else
        {
            await MainWindow.OpenPathAsync(Host.Snapshot.PackagesDirectory);
        }
    }

    private void ClearFilters()
    {
        _search = string.Empty;
        _filter = PackageFilter.All;
        _rendering = true;
        SearchBox.Text = string.Empty;
        if (FilterComboBox.ItemsSource is IEnumerable<PackageFilterOption> options)
        {
            FilterComboBox.SelectedItem = options.First(option => option.Value == PackageFilter.All);
        }
        _rendering = false;
        ApplyFilters();
    }

    private async void PackageToggle_Toggled(object sender, RoutedEventArgs e)
    {
        if (_rendering
            || sender is not ToggleSwitch { DataContext: PackageRowViewModel row } toggle
            || _snapshot is null)
        {
            return;
        }
        var currentlyEnabled = !_snapshot.DisabledPackageIds.Contains(row.Id);
        if (toggle.IsOn != currentlyEnabled)
        {
            await Host.RunBackendAsync(
                "setPackageEnabled",
                new { packageID = row.Id, enabled = toggle.IsOn });
        }
    }

    private async void PriorityTextBox_KeyDown(object sender, KeyRoutedEventArgs e)
    {
        if (e.Key == VirtualKey.Enter && sender is TextBox textBox)
        {
            await ApplyPriorityAsync(textBox);
        }
    }

    private async void PriorityTextBox_LostFocus(object sender, RoutedEventArgs e)
    {
        if (sender is TextBox textBox)
        {
            await ApplyPriorityAsync(textBox);
        }
    }

    private async Task ApplyPriorityAsync(TextBox textBox)
    {
        if (textBox.DataContext is not PackageRowViewModel row)
        {
            return;
        }
        if (!int.TryParse(textBox.Text, NumberStyles.Integer, CultureInfo.InvariantCulture, out var priority))
        {
            textBox.Text = row.PriorityText;
            return;
        }
        if (priority == row.Package.Priority)
        {
            return;
        }
        textBox.IsEnabled = false;
        await Host.RunBackendAsync("setPackagePriority", new { packageID = row.Id, priority });
    }

    private async void ResetPriorityButton_Click(object sender, RoutedEventArgs e)
    {
        if (sender is Button { DataContext: PackageRowViewModel row })
        {
            await Host.RunBackendAsync(
                "setPackagePriority",
                new { packageID = row.Id, priority = (int?)null });
        }
    }

    private async void InstallDependenciesButton_Click(object sender, RoutedEventArgs e)
    {
        if (sender is Button { DataContext: PackageRowViewModel row })
        {
            await Host.RunBackendAsync("installMissingDependencies", new { packageID = row.Id });
        }
    }

    private async void EnableDependenciesButton_Click(object sender, RoutedEventArgs e)
    {
        if (sender is Button { DataContext: PackageRowViewModel row })
        {
            await Host.RunBackendAsync("enableDependencies", new { packageID = row.Id });
        }
    }

    private async void UpdatePackageButton_Click(object sender, RoutedEventArgs e)
    {
        if (sender is Button { DataContext: PackageRowViewModel row })
        {
            await Host.RunBackendAsync("updateManagedPackage", new { packageID = row.Id });
        }
    }

    private async void OpenPackageDirectoryButton_Click(object sender, RoutedEventArgs e)
    {
        if (sender is Button { DataContext: PackageRowViewModel row })
        {
            await MainWindow.OpenPathAsync(row.Package.Directory);
        }
    }

    private async void BuildPackageButton_Click(object sender, RoutedEventArgs e)
    {
        if (sender is Button { DataContext: PackageRowViewModel row } button)
        {
            button.IsEnabled = false;
            await Host.RunBackendAsync("buildPackage", new { packageID = row.Id });
        }
    }

    private MainWindow Host => _host ?? throw new InvalidOperationException("Packages page is not attached.");
}

internal sealed class PackageRowViewModel
{
    internal PackageRowViewModel(MainWindow host, BackendAppSnapshot snapshot, PackageView package)
    {
        Package = package;
        Id = package.Id;
        DisplayName = package.DisplayName;
        VersionText = host.Text(
            PresentationTextKey.PackagesSourceVersion,
            ("version", package.DisplayVersion));
        Detail = string.IsNullOrWhiteSpace(package.Detail)
            ? host.Text(PresentationTextKey.PackagesNoDescription)
            : package.Detail;
        StatusTitle = package.Presentation.StatusTitle;
        StatusDetail = package.Presentation.StatusDetail;
        StatusBrush = host.ToneBrush(package.Presentation.StatusTone);
        StatusDetailVisibility = string.IsNullOrWhiteSpace(StatusDetail)
            ? Visibility.Collapsed
            : Visibility.Visible;
        IsEnabled = !snapshot.DisabledPackageIds.Contains(package.Id);
        CanSetEnabled = package.AvailableActions.SetEnabled;
        PriorityLabel = host.Text(PresentationTextKey.PackagesPriority);
        PriorityText = package.Priority.ToString(CultureInfo.InvariantCulture);
        CanSetPriority = package.AvailableActions.SetPriority;
        ResetPriorityLabel = host.Text(
            PresentationTextKey.PackagesResetPriority,
            ("priority", package.DeclaredPriority.ToString(CultureInfo.InvariantCulture)));
        ResetPriorityVisibility = package.PriorityOverride is null ? Visibility.Collapsed : Visibility.Visible;
        InstallDependenciesLabel = host.Text(PresentationTextKey.PackagesInstallDependencies);
        InstallDependenciesVisibility = package.CanInstallMissingDependencies
            ? Visibility.Visible
            : Visibility.Collapsed;
        CanInstallDependencies = package.AvailableActions.InstallMissingDependencies;
        EnableDependenciesLabel = host.Text(PresentationTextKey.PackagesEnableDependencies);
        EnableDependenciesVisibility = package.CanEnableDependencies
            ? Visibility.Visible
            : Visibility.Collapsed;
        CanEnableDependencies = package.AvailableActions.EnableDependencies;
        var remoteUpdateAvailable = snapshot.RemotePackageUpdates.TryGetValue(package.Id, out var remote)
                                    && remote.Status == "available";
        UpdateLabel = host.Text(PresentationTextKey.PackagesUpdateAndBuild);
        UpdateVisibility = remoteUpdateAvailable ? Visibility.Visible : Visibility.Collapsed;
        CanUpdate = package.AvailableActions.UpdateManagedPackage;
        OpenDirectoryLabel = host.Text(PresentationTextKey.PackagesOpenDirectory);
        CanOpenDirectory = package.AvailableActions.OpenDirectory;
        BuildLabel = host.Text(snapshot.BuildingPackageIds.Contains(package.Id)
            ? PresentationTextKey.PackagesBuilding
            : package.BuildDisposition == "current"
                ? PresentationTextKey.PackagesRebuild
                : PresentationTextKey.PackagesBuild);
        CanBuild = package.AvailableActions.Build;
        MessagesText = string.Join(Environment.NewLine, host.PackageMessages(package));
        MessagesVisibility = string.IsNullOrWhiteSpace(MessagesText)
            ? Visibility.Collapsed
            : Visibility.Visible;
    }

    internal PackageView Package { get; }
    public string Id { get; }
    public string DisplayName { get; }
    public string VersionText { get; }
    public string Detail { get; }
    public string StatusTitle { get; }
    public string StatusDetail { get; }
    public Brush StatusBrush { get; }
    public Visibility StatusDetailVisibility { get; }
    public bool IsEnabled { get; }
    public bool CanSetEnabled { get; }
    public string PriorityLabel { get; }
    public string PriorityText { get; }
    public bool CanSetPriority { get; }
    public string ResetPriorityLabel { get; }
    public Visibility ResetPriorityVisibility { get; }
    public string InstallDependenciesLabel { get; }
    public Visibility InstallDependenciesVisibility { get; }
    public bool CanInstallDependencies { get; }
    public string EnableDependenciesLabel { get; }
    public Visibility EnableDependenciesVisibility { get; }
    public bool CanEnableDependencies { get; }
    public string UpdateLabel { get; }
    public Visibility UpdateVisibility { get; }
    public bool CanUpdate { get; }
    public string OpenDirectoryLabel { get; }
    public bool CanOpenDirectory { get; }
    public string BuildLabel { get; }
    public bool CanBuild { get; }
    public string MessagesText { get; }
    public Visibility MessagesVisibility { get; }
}
