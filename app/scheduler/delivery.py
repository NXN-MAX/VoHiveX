"""Observe result by exact message ID; never infer success from SMS history order."""
import json
import threading


def classify(result, require_counts=True):
    state = result.get('state', result.get('delivery_state', ''))
    if state == 'failed':
        return 'failed', '短信发送失败，短信状态接口已确认失败。'
    if state == 'timeout':
        return 'unknown', '短信确认超时，不能确定是否发送成功，请核对短信中心。'
    if state == 'acked':
        if require_counts:
            total, acks = result.get('parts_total'), result.get('acks')
            if type(total) is not int or type(acks) is not int or total <= 0 or acks < total:
                return None
        return 'success', '短信发送成功，全部分段已获网络确认；不代表对方已收到或已阅读。'
    return None


class DeliveryWorker:
    def __init__(self, store, query, stop=None):
        self.store, self.query = store, query
        self.stop = stop if stop is not None else threading.Event()

    def tick(self):
        row = self.store.next_delivery()
        if not row:
            return
        event = json.loads(row['event'])
        sent = event.get('send_result') or {}
        message_id = sent.get('message_id')
        if event['status'] == 'failed':
            result = ('failed', event['detail'])
        elif event['status'] != 'success':
            result = ('unknown', '发送请求结果不确定，无法安全关联短信状态，未自动重发。')
        elif not message_id:
            result = classify(sent, require_counts=False)
            if result and result[0] == 'success':
                result = ('success', '短信发送成功，发信接口已确认；该通道没有可跟踪的消息ID，不能确认对方是否收到。')
            if not result:
                result = ('unknown', '短信已提交，但该通道没有返回可查询的消息ID或明确发送状态。')
        else:
            result = None
            try:
                value = self.query(message_id)
                # Fail closed on stale, missing or mismatched identifiers/devices.
                if (isinstance(value, dict) and value.get('message_id') == message_id
                        and (not value.get('device_id') or value['device_id'] == event['device_id'])):
                    result = classify(value)
            except Exception:
                pass  # A failed query is not evidence that the SMS failed.
            if result is None and int(self.store.clock()) >= row['deadline']:
                result = ('unknown', '24小时内未获取到明确发送结果，已停止查询；未自动重发短信。')
        if result:
            self.store.finish_delivery(row['run_id'], *result)
        else:
            self.store.defer_delivery(row['run_id'])

    def run(self):
        while not self.stop.is_set():
            try:
                self.tick()
            except Exception as exc:
                print('SMS result observer failed:', type(exc).__name__, flush=True)
            self.stop.wait(0.25)
