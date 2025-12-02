# 빠른 해결 방법

## 1. Prometheus UI에서 직접 찾기

Prometheus UI (http://localhost:9090/rules)에서:

1. **검색 필터를 비우기** (필터 제거)
2. **스크롤해서 `alert-rule-operator` 그룹 찾기**
   - 그룹 이름: `alert-rule-operator`
   - 규칙 이름: `test-appPodDown`

또는:

1. **Alerts 탭** (http://localhost:9090/alerts)으로 이동
2. 검색창에 `test-appPodDown` 입력

## 2. 레이블 추가 확인

```bash
# 레이블이 제대로 추가되었는지 확인
kubectl get prometheusrules test-app-alert -n default --show-labels
```

출력에 `release=monitoring`이 있어야 합니다.

## 3. Prometheus Operator 재시작 (필요시)

```bash
# Prometheus Operator가 규칙을 다시 스캔하도록 유도
kubectl delete pod -n default -l app.kubernetes.io/name=prometheus-operator
```

## 4. 오퍼레이터 재배포 (코드 수정 반영)

```bash
# 이미 수정된 코드로 재배포
make docker-build IMG=controller:latest
make deploy IMG=controller:latest
```

재배포 후 새로 생성되는 PrometheusRule에는 자동으로 `release: monitoring` 레이블이 추가됩니다.

## 5. 즉시 테스트

새로운 Deployment를 생성해서 테스트:

```bash
kubectl create deployment test-app2 --image=nginx:latest
# 잠시 후
kubectl get prometheusrules test-app2-alert -n default --show-labels
# release=monitoring 레이블이 있어야 함
```

