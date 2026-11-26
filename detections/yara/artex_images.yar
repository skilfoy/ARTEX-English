rule ARTEX_Known_Bad_Image_Name
{
    meta:
        description = "File references a prebuilt ARTEX image reported backdoored in November 2026"
        author = "skilfoy"
        date = "2026-11-26"
        reference = "https://github.com/skilfoy/ARTEX-English/blob/next/detections/README.md"

    strings:
        $tomicdd = "tomicdd/artex" nocase
        $congdong = "congdonglinux/artex" nocase

    condition:
        any of them
}
