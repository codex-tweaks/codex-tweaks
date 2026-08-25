using CodexTweaks.Windows.Generated;
using CodexTweaks.Windows.Models;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace CodexTweaks.Windows.Pages;

public sealed partial class LogsPage : Page
{
    private MainWindow? _host;

    public LogsPage()
    {
        InitializeComponent();
    }

    internal void Render(MainWindow host, BackendAppSnapshot snapshot)
    {
        _host = host;
        PageTitle.Text = host.Text(PresentationTextKey.LogsTitle);
        PageSubtitle.Text = host.Text(PresentationTextKey.LogsSubtitle);
        LogPathText.Text = snapshot.LogPath;
        var logText = string.IsNullOrWhiteSpace(snapshot.LogText)
            ? host.Text(PresentationTextKey.LogsEmpty)
            : snapshot.LogText;
        if (!string.Equals(LogTextBox.Text, logText, StringComparison.Ordinal))
        {
            LogTextBox.Text = logText;
        }
        RefreshButton.Label = host.Text(PresentationTextKey.LogsRefresh);
        RefreshButton.IsEnabled = snapshot.Presentation.Actions.RefreshLog;
        OpenFileButton.Label = host.Text(PresentationTextKey.LogsOpenFile);
        OpenFileButton.IsEnabled = snapshot.Presentation.Actions.OpenLogFile;
        ClearButton.Label = host.Text(PresentationTextKey.LogsClear);
        ClearButton.IsEnabled = snapshot.Presentation.Actions.ClearLog;
    }

    private async void RefreshButton_Click(object sender, RoutedEventArgs e) =>
        await Host.RunBackendAsync("refreshLog");

    private async void OpenFileButton_Click(object sender, RoutedEventArgs e) =>
        await MainWindow.OpenPathAsync(Host.Snapshot.LogPath);

    private async void ClearButton_Click(object sender, RoutedEventArgs e) =>
        await Host.ConfirmClearLogAsync();

    private MainWindow Host => _host ?? throw new InvalidOperationException("Logs page is not attached.");
}
