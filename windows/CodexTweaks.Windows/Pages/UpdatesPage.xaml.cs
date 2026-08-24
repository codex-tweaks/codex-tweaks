using CodexTweaks.Windows.Generated;
using CodexTweaks.Windows.Models;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace CodexTweaks.Windows.Pages;

public sealed partial class UpdatesPage : Page
{
    private MainWindow? _host;
    private BackendAppSnapshot? _snapshot;
    private bool _rendering;

    public UpdatesPage()
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
            PageTitle.Text = host.Text(PresentationTextKey.UpdateTitle);
            PageSubtitle.Text = host.Text(PresentationTextKey.UpdateSubtitle);
            ApplicationTitle.Text = host.Text(PresentationTextKey.AppName);
            ApplicationVersion.Text = host.Text(
                PresentationTextKey.UpdateVersionBuild,
                ("version", snapshot.Update.CurrentVersion),
                ("build", snapshot.Update.BuildNumber));
            RepositoryButtonText.Text = host.Text(PresentationTextKey.UpdateRepository);
            RepositoryButton.IsEnabled = snapshot.Presentation.Actions.OpenRepository;

            SoftwareUpdateTitle.Text = host.Text(PresentationTextKey.UpdateSoftwareUpdate);
            ChannelTitle.Text = host.Text(PresentationTextKey.UpdateChannel);
            ChannelDetail.Text = host.Text(snapshot.Update.Channel == "beta"
                ? PresentationTextKey.UpdateChannelBetaDetail
                : PresentationTextKey.UpdateChannelStableDetail);
            ChannelComboBox.Items.Clear();
            ChannelComboBox.Items.Add(new ComboBoxItem
            {
                Content = host.Text(PresentationTextKey.UpdateChannelStable),
                Tag = "stable",
            });
            ChannelComboBox.Items.Add(new ComboBoxItem
            {
                Content = host.Text(PresentationTextKey.UpdateChannelBeta),
                Tag = "beta",
            });
            ChannelComboBox.SelectedIndex = snapshot.Update.Channel == "beta" ? 1 : 0;
            ChannelComboBox.IsEnabled = snapshot.Presentation.Actions.SetUpdatePreferences
                && !host.CheckingUpdate
                && !host.ApplyingUpdate;

            AutoCheckTitle.Text = host.Text(PresentationTextKey.UpdateAutoCheck);
            AutoCheckToggle.IsOn = snapshot.Update.AutoCheck;
            AutoCheckToggle.IsEnabled = snapshot.Presentation.Actions.SetUpdatePreferences
                && !host.CheckingUpdate
                && !host.ApplyingUpdate;

            CurrentVersionLabel.Text = host.Text(PresentationTextKey.UpdateCurrentVersion);
            CurrentVersionValue.Text = snapshot.Update.CurrentVersion;
            LatestVersionLabel.Text = host.Text(PresentationTextKey.UpdateLatestVersion);
            LatestVersionValue.Text = snapshot.Update.LatestVersionString;
            LastCheckLabel.Text = host.Text(PresentationTextKey.UpdateLastCheck);
            LastCheckValue.Text = host.FormatLastCheck(snapshot.Update.LastCheckAt);

            RenderStatus(host, snapshot);

            CheckButtonText.Text = host.Text(host.CheckingUpdate || host.ApplyingUpdate
                ? PresentationTextKey.UpdateChecking
                : PresentationTextKey.UpdateCheck);
            CheckButton.IsEnabled = snapshot.Presentation.Actions.CheckAppUpdate
                && !host.CheckingUpdate
                && !host.ApplyingUpdate;

            InstallButton.Visibility = host.VelopackResult is { Installed: true, Version: not null }
                ? Visibility.Visible
                : Visibility.Collapsed;
            InstallButtonText.Text = host.VelopackResult?.Version is { } version
                ? host.Text(host.ApplyingUpdate
                    ? PresentationTextKey.UpdateApplyProgress
                    : PresentationTextKey.UpdateInstall, ("version", version))
                : string.Empty;
            InstallButton.IsEnabled = snapshot.Presentation.Actions.InstallAppUpdate
                && !host.CheckingUpdate
                && !host.ApplyingUpdate;

            ReleaseButton.Visibility = host.VelopackResult is not { Installed: true, Version: not null }
                                       && snapshot.Update.LatestRelease?.HtmlUrl is not null
                ? Visibility.Visible
                : Visibility.Collapsed;
            ReleaseButtonText.Text = host.Text(PresentationTextKey.UpdateViewRelease);
        }
        finally
        {
            _rendering = false;
        }
    }

    private void RenderStatus(MainWindow host, BackendAppSnapshot snapshot)
    {
        UpdateStatusInfo.IsOpen = false;
        if (host.CheckingUpdate)
        {
            UpdateStatusInfo.Severity = InfoBarSeverity.Informational;
            UpdateStatusInfo.Title = host.Text(PresentationTextKey.UpdateChecking);
            UpdateStatusInfo.Message = string.Empty;
            UpdateStatusInfo.IsOpen = true;
            return;
        }
        if (!string.IsNullOrWhiteSpace(snapshot.Update.LastError))
        {
            UpdateStatusInfo.Severity = InfoBarSeverity.Error;
            UpdateStatusInfo.Title = host.Text(PresentationTextKey.StatusErrorTitle);
            UpdateStatusInfo.Message = snapshot.Update.LastError;
            UpdateStatusInfo.IsOpen = true;
            return;
        }
        if (!string.IsNullOrWhiteSpace(host.VelopackResult?.Error))
        {
            UpdateStatusInfo.Severity = InfoBarSeverity.Error;
            UpdateStatusInfo.Title = host.Text(PresentationTextKey.StatusErrorTitle);
            UpdateStatusInfo.Message = host.Text(
                PresentationTextKey.UpdateCheckFailed,
                ("message", host.VelopackResult!.Error!));
            UpdateStatusInfo.IsOpen = true;
            return;
        }
        if (host.VelopackResult?.Installed == false)
        {
            UpdateStatusInfo.Severity = InfoBarSeverity.Warning;
            UpdateStatusInfo.Title = host.Text(PresentationTextKey.UpdateNotInstalled);
            UpdateStatusInfo.Message = string.Empty;
            UpdateStatusInfo.IsOpen = true;
            return;
        }
        if (host.VelopackResult?.Version is not null)
        {
            UpdateStatusInfo.Severity = InfoBarSeverity.Success;
            UpdateStatusInfo.Title = host.Text(PresentationTextKey.UpdateAvailable);
            UpdateStatusInfo.Message = host.VelopackResult.Version;
            UpdateStatusInfo.IsOpen = true;
            return;
        }
        if (host.VelopackResult is not null)
        {
            UpdateStatusInfo.Severity = InfoBarSeverity.Informational;
            UpdateStatusInfo.Title = host.Text(PresentationTextKey.UpdateCurrent);
            UpdateStatusInfo.Message = host.Text(PresentationTextKey.UpdateNoneAvailable);
            UpdateStatusInfo.IsOpen = true;
        }
    }

    private async void ChannelComboBox_SelectionChanged(object sender, SelectionChangedEventArgs e)
    {
        if (_rendering
            || _host is null
            || _snapshot is null
            || ChannelComboBox.SelectedItem is not ComboBoxItem { Tag: string channel }
            || channel == _snapshot.Update.Channel)
        {
            return;
        }
        await _host.RunBackendAsync("setUpdateChannel", new { channel });
    }

    private async void AutoCheckToggle_Toggled(object sender, RoutedEventArgs e)
    {
        if (_rendering
            || _host is null
            || _snapshot is null
            || AutoCheckToggle.IsOn == _snapshot.Update.AutoCheck)
        {
            return;
        }
        await _host.RunBackendAsync("setUpdateAutoCheck", new { enabled = AutoCheckToggle.IsOn });
    }

    private async void RepositoryButton_Click(object sender, RoutedEventArgs e) =>
        await MainWindow.OpenPathAsync(Host.Snapshot.Presentation.Platform.RepositoryURL);

    private async void CheckButton_Click(object sender, RoutedEventArgs e) =>
        await Host.CheckUpdatesAsync();

    private async void InstallButton_Click(object sender, RoutedEventArgs e) =>
        await Host.InstallUpdateAsync();

    private async void ReleaseButton_Click(object sender, RoutedEventArgs e)
    {
        if (Host.Snapshot.Update.LatestRelease?.HtmlUrl is { } releaseUrl)
        {
            await MainWindow.OpenPathAsync(releaseUrl);
        }
    }

    private MainWindow Host => _host ?? throw new InvalidOperationException("Updates page is not attached.");
}
