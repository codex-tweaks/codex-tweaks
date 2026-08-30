using System.Globalization;
using CodexTweaks.Windows.Generated;
using CodexTweaks.Windows.Models;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace CodexTweaks.Windows.Pages;

public sealed partial class OverviewPage : Page
{
    private MainWindow? _host;
    private BackendAppSnapshot? _snapshot;
    private bool _rendering;

    public OverviewPage()
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
            PageTitle.Text = host.Text(PresentationTextKey.OverviewTitle);
            PageSubtitle.Text = host.Text(PresentationTextKey.OverviewSubtitle);
            StatusTitle.Text = snapshot.Presentation.Status.Title;
            StatusTitle.Foreground = host.ToneBrush(snapshot.Presentation.Status.Tone);
            StatusIcon.Foreground = StatusTitle.Foreground;
            StatusIconContainer.Background = host.ToneBackgroundBrush(snapshot.Presentation.Status.Tone);
            StatusIcon.Symbol = snapshot.Presentation.Status.Tone switch
            {
                "success" => Symbol.Accept,
                "warning" => Symbol.Important,
                "danger" => Symbol.Cancel,
                "neutral" => Symbol.Help,
                _ => Symbol.Sync,
            };
            StatusDetail.Text = snapshot.Presentation.Status.Detail;
            StatusDetail.Visibility = string.IsNullOrWhiteSpace(StatusDetail.Text)
                ? Visibility.Collapsed
                : Visibility.Visible;
            PackageSummary.Text = host.Text(
                PresentationTextKey.PackagesEnabledSummary,
                ("enabled", snapshot.EnabledPackageCount.ToString(CultureInfo.InvariantCulture)),
                ("total", snapshot.Packages.Count.ToString(CultureInfo.InvariantCulture)),
                ("active", snapshot.ActivePackageCount.ToString(CultureInfo.InvariantCulture)));

            ControlTitle.Text = host.Text(PresentationTextKey.OverviewControl);
            EnableTitle.Text = host.Text(PresentationTextKey.OverviewEnable);
            EnableDetail.Text = host.Text(PresentationTextKey.OverviewEnableDetail);
            EnableToggle.IsOn = snapshot.Enabled;
            EnableToggle.IsEnabled = snapshot.Presentation.Actions.SetEnabled;
            DisableGPUAccelerationTitle.Text = host.Text(PresentationTextKey.OverviewDisableGPUAcceleration);
            DisableGPUAccelerationDetail.Text = host.Text(PresentationTextKey.OverviewDisableGPUAccelerationDetail);
            DisableGPUAccelerationToggle.IsOn = snapshot.DisableGPUAcceleration;
            DisableGPUAccelerationToggle.IsEnabled = snapshot.Presentation.Actions.SetDisableGPUAcceleration;
            OpenCodexButtonText.Text = host.Text(snapshot.Presentation.Actions.OpenCodex
                ? PresentationTextKey.OverviewOpenCodex
                : PresentationTextKey.StatusLaunchingCodexTitle);
            OpenCodexButton.IsEnabled = snapshot.Presentation.Actions.OpenCodex;
            OpenCodexButton.Visibility = snapshot.Presentation.Actions.RestartCodex
                ? Visibility.Collapsed
                : Visibility.Visible;
            OpenCodexButton.Style = snapshot.Status.Kind == "connected"
                ? null
                : Application.Current.Resources["AccentButtonStyle"] as Style;
            RestartButtonText.Text = host.Text(PresentationTextKey.OverviewRestartAndConnect);
            RestartButton.Visibility = snapshot.Presentation.Actions.RestartCodex
                ? Visibility.Visible
                : Visibility.Collapsed;
            RestartCodexUITitle.Text = host.Text(PresentationTextKey.OverviewRestartCodexUI);
            RestartCodexUIDetail.Text = host.Text(PresentationTextKey.OverviewRestartCodexUIDetail);
            RestartCodexUIButtonText.Text = host.Text(PresentationTextKey.OverviewRestartCodexUI);
            RestartCodexUIButton.IsEnabled = snapshot.Presentation.Actions.RestartCodexUI;
            ReinjectButtonText.Text = host.Text(PresentationTextKey.OverviewReinject);
            ReinjectButton.IsEnabled = snapshot.Presentation.Actions.Reinject;
            ManagePackagesButtonText.Text = host.Text(PresentationTextKey.OverviewManagePackages);
            ViewLogsButtonText.Text = host.Text(PresentationTextKey.OverviewViewLogs);

            AuthoringTitle.Text = host.Text(PresentationTextKey.OverviewAiAuthoring);
            AuthoringActionTitle.Text = host.Text(PresentationTextKey.OverviewCopySkill);
            AuthoringDetail.Text = host.Text(PresentationTextKey.OverviewCopySkillDetail);
            CopySkillButtonText.Text = host.Text(PresentationTextKey.OverviewCopy);
            CopySkillButton.IsEnabled = snapshot.Presentation.Actions.ReadAuthoringPrompt;

            ResourcesTitle.Text = host.Text(PresentationTextKey.OverviewConnection);
            CdpEndpointLabel.Text = host.Text(PresentationTextKey.OverviewCdpEndpoint);
            CdpEndpointValue.Text = snapshot.Presentation.Platform.CdpEndpoint;
            InjectionScopeLabel.Text = host.Text(PresentationTextKey.OverviewInjectionScope);
            InjectionScopeValue.Text = host.Text(PresentationTextKey.OverviewAppPagesOnly);
            RefreshPolicyLabel.Text = host.Text(PresentationTextKey.OverviewRefreshPolicy);
            RefreshPolicyValue.Text = host.Text(PresentationTextKey.OverviewRefreshEveryTwoSeconds);
            LoadOrderLabel.Text = host.Text(PresentationTextKey.OverviewLoadOrder);
            LoadOrderValue.Text = host.Text(PresentationTextKey.OverviewLoadOrderDetail);
            PackagesPathLabel.Text = host.Text(PresentationTextKey.OverviewResources);
            PackagesPath.Text = snapshot.PackagesDirectory;
            OpenPackagesDirectoryButtonText.Text = host.Text(PresentationTextKey.OverviewOpenPackagesDirectory);
            OpenPackagesDirectoryButton.IsEnabled = snapshot.Presentation.Actions.OpenPackagesDirectory;
        }
        finally
        {
            _rendering = false;
        }
    }

    private async void EnableToggle_Toggled(object sender, RoutedEventArgs e)
    {
        if (_rendering || _host is null || _snapshot is null || EnableToggle.IsOn == _snapshot.Enabled)
        {
            return;
        }
        await _host.RunBackendAsync("setEnabled", new { enabled = EnableToggle.IsOn });
    }

    private async void DisableGPUAccelerationToggle_Toggled(object sender, RoutedEventArgs e)
    {
        if (_rendering || _host is null || _snapshot is null
            || DisableGPUAccelerationToggle.IsOn == _snapshot.DisableGPUAcceleration)
        {
            return;
        }
        await _host.RunBackendAsync(
            "setDisableGPUAcceleration",
            new { enabled = DisableGPUAccelerationToggle.IsOn });
    }

    private async void OpenCodexButton_Click(object sender, RoutedEventArgs e) =>
        await RunButtonAsync(OpenCodexButton, () => Host.RunBackendAsync("openCodex"));

    private async void RestartButton_Click(object sender, RoutedEventArgs e) =>
        await RunButtonAsync(RestartButton, Host.ConfirmRestartAsync);

    private async void RestartCodexUIButton_Click(object sender, RoutedEventArgs e) =>
        await RunButtonAsync(RestartCodexUIButton, Host.ConfirmRestartCodexUIAsync);

    private async void ReinjectButton_Click(object sender, RoutedEventArgs e) =>
        await RunButtonAsync(ReinjectButton, () => Host.RunBackendAsync("reinject"));

    private async void ManagePackagesButton_Click(object sender, RoutedEventArgs e) =>
        await Host.NavigateAsync("packages");

    private async void ViewLogsButton_Click(object sender, RoutedEventArgs e) =>
        await Host.NavigateAsync("logs");

    private async void CopySkillButton_Click(object sender, RoutedEventArgs e) =>
        await RunButtonAsync(CopySkillButton, () => Host.CopyAuthoringPromptAsync());

    private async void OpenPackagesDirectoryButton_Click(object sender, RoutedEventArgs e) =>
        await MainWindow.OpenPathAsync(Host.Snapshot.PackagesDirectory);

    private MainWindow Host => _host ?? throw new InvalidOperationException("Overview page is not attached.");

    private static async Task RunButtonAsync(Button button, Func<Task> action)
    {
        button.IsEnabled = false;
        try
        {
            await action();
        }
        finally
        {
            if (button.XamlRoot is not null)
            {
                button.IsEnabled = true;
            }
        }
    }
}
