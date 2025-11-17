BEGIN {
  in_yaml = 0
  seen_heading = 0
}

NR == 1 && $0 == "---" {
  in_yaml = 1
  next
}

in_yaml && $0 == "---" {
  in_yaml = 0
  next
}

in_yaml {
  next
}

!seen_heading && $0 ~ /^# / {
  seen_heading = 1
  next
}

{
  print
}
