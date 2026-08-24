using System.Collections.ObjectModel;
using System.ComponentModel;
using System.Globalization;
using System.Runtime.CompilerServices;
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
    private readonly Dictionary<string, PackageRowViewModel> _rowsById = new(StringComparer.Ordinal);
    private readonly ObservableCollection<PackageRowViewModel> _visibleRows = [];
    private PackageFilter _filter = PackageFilter.All;
    private string _search = string.Empty;
    private bool _rendering;

    public PackagesPage()
    {
        InitializeComponent();
        PackagesList.ItemsSource = _visibleRows;
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

            UpdateRows(host, snapshot);
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

    private void UpdateRows(MainWindow host, BackendAppSnapshot snapshot)
    {
        var currentPackageIds = snapshot.Packages
            .Select(package => package.Id)
            .ToHashSet(StringComparer.Ordinal);
        foreach (var packageId in _rowsById.Keys
                     .Where(packageId => !currentPackageIds.Contains(packageId))
                     .ToList())
        {
            _rowsById.Remove(packageId);
        }

        var rows = new List<PackageRowViewModel>(snapshot.Packages.Count);
        foreach (var package in snapshot.Packages)
        {
            if (!_rowsById.TryGetValue(package.Id, out var row))
            {
                row = new PackageRowViewModel(package.Id);
                _rowsById.Add(package.Id, row);
            }
            row.Update(host, snapshot, package);
            rows.Add(row);
        }
        _allRows = rows;
    }

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

        SynchronizeVisibleRows(filtered);
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

    private void SynchronizeVisibleRows(IReadOnlyList<PackageRowViewModel> rows)
    {
        // Preserve row identity so periodic backend snapshots do not recreate active controls.
        for (var index = 0; index < rows.Count; index++)
        {
            var row = rows[index];
            if (index < _visibleRows.Count && ReferenceEquals(_visibleRows[index], row))
            {
                continue;
            }

            var currentIndex = _visibleRows.IndexOf(row);
            if (currentIndex >= 0)
            {
                _visibleRows.Move(currentIndex, index);
            }
            else
            {
                _visibleRows.Insert(index, row);
            }
        }
        while (_visibleRows.Count > rows.Count)
        {
            _visibleRows.RemoveAt(_visibleRows.Count - 1);
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
        if (toggle.IsOn == currentlyEnabled || row.EnablementPending)
        {
            return;
        }

        var requestedEnabled = toggle.IsOn;
        row.BeginEnablementChange(requestedEnabled);
        var error = await Host.RunBackendAsync(
            "setPackageEnabled",
            new { packageID = row.Id, enabled = requestedEnabled });
        row.EndEnablementChange(error is null);
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
        if (sender is Button { DataContext: PackageRowViewModel row })
        {
            await Host.RunBackendAsync("buildPackage", new { packageID = row.Id });
        }
    }

    private MainWindow Host => _host ?? throw new InvalidOperationException("Packages page is not attached.");
}

internal sealed class PackageRowViewModel : INotifyPropertyChanged
{
    private bool _canSetEnabledFromSnapshot;
    private bool _enablementPending;
    private bool _requestedEnabled;
    private bool _snapshotEnabled;
    private string _displayName = string.Empty;
    private string _versionText = string.Empty;
    private string _detail = string.Empty;
    private string _statusTitle = string.Empty;
    private string _statusDetail = string.Empty;
    private Brush _statusBrush = null!;
    private Visibility _statusDetailVisibility;
    private bool _isEnabled;
    private bool _canSetEnabled;
    private string _priorityLabel = string.Empty;
    private string _priorityText = string.Empty;
    private bool _canSetPriority;
    private string _resetPriorityLabel = string.Empty;
    private Visibility _resetPriorityVisibility;
    private string _installDependenciesLabel = string.Empty;
    private Visibility _installDependenciesVisibility;
    private bool _canInstallDependencies;
    private string _enableDependenciesLabel = string.Empty;
    private Visibility _enableDependenciesVisibility;
    private bool _canEnableDependencies;
    private string _updateLabel = string.Empty;
    private Visibility _updateVisibility;
    private bool _canUpdate;
    private string _openDirectoryLabel = string.Empty;
    private bool _canOpenDirectory;
    private string _buildLabel = string.Empty;
    private bool _canBuild;
    private string _messagesText = string.Empty;
    private Visibility _messagesVisibility;

    internal PackageRowViewModel(string id)
    {
        Id = id;
    }

    public event PropertyChangedEventHandler? PropertyChanged;

    internal PackageView Package { get; private set; } = null!;
    internal bool EnablementPending => _enablementPending;
    public string Id { get; }
    public string DisplayName { get => _displayName; private set => SetProperty(ref _displayName, value); }
    public string VersionText { get => _versionText; private set => SetProperty(ref _versionText, value); }
    public string Detail { get => _detail; private set => SetProperty(ref _detail, value); }
    public string StatusTitle { get => _statusTitle; private set => SetProperty(ref _statusTitle, value); }
    public string StatusDetail { get => _statusDetail; private set => SetProperty(ref _statusDetail, value); }
    public Brush StatusBrush { get => _statusBrush; private set => SetProperty(ref _statusBrush, value); }
    public Visibility StatusDetailVisibility { get => _statusDetailVisibility; private set => SetProperty(ref _statusDetailVisibility, value); }
    public bool IsEnabled { get => _isEnabled; private set => SetProperty(ref _isEnabled, value); }
    public bool CanSetEnabled { get => _canSetEnabled; private set => SetProperty(ref _canSetEnabled, value); }
    public string PriorityLabel { get => _priorityLabel; private set => SetProperty(ref _priorityLabel, value); }
    public string PriorityText { get => _priorityText; private set => SetProperty(ref _priorityText, value); }
    public bool CanSetPriority { get => _canSetPriority; private set => SetProperty(ref _canSetPriority, value); }
    public string ResetPriorityLabel { get => _resetPriorityLabel; private set => SetProperty(ref _resetPriorityLabel, value); }
    public Visibility ResetPriorityVisibility { get => _resetPriorityVisibility; private set => SetProperty(ref _resetPriorityVisibility, value); }
    public string InstallDependenciesLabel { get => _installDependenciesLabel; private set => SetProperty(ref _installDependenciesLabel, value); }
    public Visibility InstallDependenciesVisibility { get => _installDependenciesVisibility; private set => SetProperty(ref _installDependenciesVisibility, value); }
    public bool CanInstallDependencies { get => _canInstallDependencies; private set => SetProperty(ref _canInstallDependencies, value); }
    public string EnableDependenciesLabel { get => _enableDependenciesLabel; private set => SetProperty(ref _enableDependenciesLabel, value); }
    public Visibility EnableDependenciesVisibility { get => _enableDependenciesVisibility; private set => SetProperty(ref _enableDependenciesVisibility, value); }
    public bool CanEnableDependencies { get => _canEnableDependencies; private set => SetProperty(ref _canEnableDependencies, value); }
    public string UpdateLabel { get => _updateLabel; private set => SetProperty(ref _updateLabel, value); }
    public Visibility UpdateVisibility { get => _updateVisibility; private set => SetProperty(ref _updateVisibility, value); }
    public bool CanUpdate { get => _canUpdate; private set => SetProperty(ref _canUpdate, value); }
    public string OpenDirectoryLabel { get => _openDirectoryLabel; private set => SetProperty(ref _openDirectoryLabel, value); }
    public bool CanOpenDirectory { get => _canOpenDirectory; private set => SetProperty(ref _canOpenDirectory, value); }
    public string BuildLabel { get => _buildLabel; private set => SetProperty(ref _buildLabel, value); }
    public bool CanBuild { get => _canBuild; private set => SetProperty(ref _canBuild, value); }
    public string MessagesText { get => _messagesText; private set => SetProperty(ref _messagesText, value); }
    public Visibility MessagesVisibility { get => _messagesVisibility; private set => SetProperty(ref _messagesVisibility, value); }

    internal void Update(MainWindow host, BackendAppSnapshot snapshot, PackageView package)
    {
        Package = package;
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
        _snapshotEnabled = !snapshot.DisabledPackageIds.Contains(package.Id);
        IsEnabled = _enablementPending ? _requestedEnabled : _snapshotEnabled;
        _canSetEnabledFromSnapshot = package.AvailableActions.SetEnabled;
        CanSetEnabled = _canSetEnabledFromSnapshot && !_enablementPending;
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
        BuildLabel = snapshot.BuildingPackageIds.Contains(package.Id)
            ? host.Text(PresentationTextKey.PackagesBuilding)
            : package.BuildDisposition switch
            {
                "notBuilt" => host.Text(package.HasDependencies
                    ? PresentationTextKey.PackagesInstallAndBuild
                    : PresentationTextKey.PackagesBuild),
                "versionUpdate" => host.Text(
                    PresentationTextKey.PackagesUpdateToVersion,
                    ("version", package.DisplayVersion)),
                "dependencyUpdate" => host.Text(PresentationTextKey.PackagesSyncAndBuild),
                "sourceChanged" or "compilerUpdate" => host.Text(PresentationTextKey.PackagesUpdateBuild),
                "current" => host.Text(PresentationTextKey.PackagesRebuild),
                _ => host.Text(PresentationTextKey.PackagesCannotBuild),
            };
        CanBuild = package.AvailableActions.Build;
        MessagesText = string.Join(Environment.NewLine, host.PackageMessages(package));
        MessagesVisibility = string.IsNullOrWhiteSpace(MessagesText)
            ? Visibility.Collapsed
            : Visibility.Visible;
    }

    internal void BeginEnablementChange(bool enabled)
    {
        _enablementPending = true;
        _requestedEnabled = enabled;
        IsEnabled = enabled;
        CanSetEnabled = false;
    }

    internal void EndEnablementChange(bool succeeded)
    {
        _enablementPending = false;
        if (!succeeded)
        {
            IsEnabled = _snapshotEnabled;
        }
        CanSetEnabled = _canSetEnabledFromSnapshot;
    }

    private void SetProperty<T>(ref T storage, T value, [CallerMemberName] string? propertyName = null)
    {
        if (EqualityComparer<T>.Default.Equals(storage, value))
        {
            return;
        }
        storage = value;
        PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(propertyName));
    }
}
