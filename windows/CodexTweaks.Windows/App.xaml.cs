using Microsoft.UI.Xaml;

namespace CodexTweaks.Windows;

public partial class App : Application
{
    private Window? _window;
    private static readonly string FrontendLogPath = Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
        "Codex Tweaks",
        "Logs",
        "windows-frontend.log");

    public App()
    {
        UnhandledException += (_, args) =>
        {
            Log($"UnhandledException: {args.Exception}");
        };
        try
        {
            Log("Application startup entered.");
            InitializeComponent();
            Log("Application XAML initialized.");
        }
        catch (Exception exception)
        {
            Log($"Application constructor failed: {exception}");
            throw;
        }
    }

    protected override void OnLaunched(LaunchActivatedEventArgs args)
    {
        try
        {
            Log("Window launch entered.");
            _window = new MainWindow();
            Log("Window constructed.");
            _window.Activate();
            Log("Window activated.");
        }
        catch (Exception exception)
        {
            Log($"Window launch failed: {exception}");
            throw;
        }
    }

    internal static void Log(string message)
    {
        try
        {
            Directory.CreateDirectory(Path.GetDirectoryName(FrontendLogPath)!);
            File.AppendAllText(
                FrontendLogPath,
                $"{DateTimeOffset.Now:O} {message}{Environment.NewLine}");
        }
        catch
        {
            // Startup diagnostics must never prevent the app from launching.
        }
    }
}
