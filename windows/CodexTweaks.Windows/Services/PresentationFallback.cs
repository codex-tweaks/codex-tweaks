using CodexTweaks.Windows.Generated;

namespace CodexTweaks.Windows.Services;

internal static class PresentationFallback
{
    internal static string Text(
        string key,
        params (string Name, string Value)[] replacements)
    {
        var value = PresentationDefaults.Text.TryGetValue(key, out var provided)
            ? provided
            : key;
        foreach (var (name, replacement) in replacements)
        {
            value = value.Replace($"{{{name}}}", replacement, StringComparison.Ordinal);
        }
        return value;
    }
}
